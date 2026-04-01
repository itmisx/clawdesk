package memory

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// Embedder 文本向量化（基于 onnxruntime，纯 Go tokenizer，跨平台无 CGO）
type Embedder struct {
	mu        sync.Mutex
	cacheDir  string
	tokenizer *Tokenizer
	session   *ort.DynamicAdvancedSession
	loaded    bool
	loadErr   error
}

const embeddingDim = 384 // multilingual-e5-small 输出维度

// NewEmbedder 创建嵌入器，cacheDir 是资源释放目录（包含模型、tokenizer.json、ORT 库）
func NewEmbedder(cacheDir string) *Embedder {
	return &Embedder{
		cacheDir: cacheDir,
	}
}

// IsAvailable 检查模型和库是否都可用
func (e *Embedder) IsAvailable() bool {
	// 检查 .ready（运行时下载）或 .extracted（go:embed 释放，兼容旧版本）
	for _, marker := range []string{".ready", ".extracted"} {
		if _, err := os.Stat(filepath.Join(e.cacheDir, marker)); err == nil {
			return true
		}
	}
	return false
}

// ModelPath 返回 ONNX 模型路径
func (e *Embedder) ModelPath() string {
	return filepath.Join(e.cacheDir, "multilingual-e5-small-quantized.onnx")
}

// Embed 将文本转为归一化向量
func (e *Embedder) Embed(text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.loaded {
		e.loadModel()
	}
	if e.loadErr != nil {
		return nil, e.loadErr
	}

	// 截断过长文本
	runes := []rune(text)
	if len(runes) > 800 {
		text = string(runes[:800])
	}

	// Tokenize
	inputIDs, attentionMask := e.tokenizer.Encode(text)

	// 截断到 500 tokens（含 BOS/EOS）
	if len(inputIDs) > 500 {
		inputIDs = inputIDs[:500]
		attentionMask = attentionMask[:500]
	}

	seqLen := int64(len(inputIDs))
	shape := ort.NewShape(1, seqLen)

	// 创建 token_type_ids（全零）
	tokenTypeIDs := make([]int64, seqLen)

	// 创建输入 tensors
	inputIDsTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("创建 input_ids tensor 失败: %w", err)
	}
	defer inputIDsTensor.Destroy()

	attentionMaskTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("创建 attention_mask tensor 失败: %w", err)
	}
	defer attentionMaskTensor.Destroy()

	tokenTypeIDsTensor, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("创建 token_type_ids tensor 失败: %w", err)
	}
	defer tokenTypeIDsTensor.Destroy()

	// 创建输出 tensor: [1, seqLen, 384]
	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, seqLen, embeddingDim))
	if err != nil {
		return nil, fmt.Errorf("创建输出 tensor 失败: %w", err)
	}
	defer outputTensor.Destroy()

	// 推理
	err = e.session.Run(
		[]ort.Value{inputIDsTensor, attentionMaskTensor, tokenTypeIDsTensor},
		[]ort.Value{outputTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("ONNX 推理失败: %w", err)
	}

	// Mean pooling: 对 seq_len 维度按 attention_mask 加权平均
	hiddenStates := outputTensor.GetData() // [1 * seqLen * 384]
	result := meanPooling(hiddenStates, attentionMask, int(seqLen), embeddingDim)

	return normalize(result), nil
}

// meanPooling 对 hidden states 按 attention_mask 加权求平均
func meanPooling(hiddenStates []float32, attentionMask []int64, seqLen, dim int) []float32 {
	result := make([]float32, dim)
	var maskSum float32

	for i := 0; i < seqLen; i++ {
		mask := float32(attentionMask[i])
		maskSum += mask
		offset := i * dim
		for j := 0; j < dim; j++ {
			result[j] += hiddenStates[offset+j] * mask
		}
	}

	if maskSum > 0 {
		for j := range result {
			result[j] /= maskSum
		}
	}

	return result
}

func (e *Embedder) loadModel() {
	e.loaded = true

	if !e.IsAvailable() {
		e.loadErr = fmt.Errorf("嵌入模型或 ONNX Runtime 库不可用")
		return
	}

	// 设置 ONNX Runtime 库路径并初始化
	libPath := filepath.Join(e.cacheDir, ortLibName)
	ort.SetSharedLibraryPath(libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		e.loadErr = fmt.Errorf("初始化 ONNX Runtime 失败: %w", err)
		return
	}

	// 加载 tokenizer
	tokenizerPath := filepath.Join(e.cacheDir, "tokenizer.json")
	tokenizerData, err := os.ReadFile(tokenizerPath)
	if err != nil {
		e.loadErr = fmt.Errorf("读取 tokenizer.json 失败: %w", err)
		return
	}
	tok, err := NewTokenizer(tokenizerData)
	if err != nil {
		e.loadErr = fmt.Errorf("初始化 tokenizer 失败: %w", err)
		return
	}
	e.tokenizer = tok

	// 创建 ONNX 推理 session（动态形状，支持不同序列长度）
	modelPath := e.ModelPath()
	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		nil,
	)
	if err != nil {
		e.loadErr = fmt.Errorf("创建 ONNX session 失败: %w", err)
		return
	}
	e.session = session
}

// Close 释放资源
func (e *Embedder) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session != nil {
		e.session.Destroy()
		e.session = nil
	}
	if e.loaded && e.loadErr == nil {
		ort.DestroyEnvironment()
	}
}

func normalize(vec []float32) []float32 {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	norm := float32(1.0 / math.Sqrt(sum))
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = v * norm
	}
	return result
}
