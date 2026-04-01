package memory

import (
	"math"
	"testing"
)

func setupTestEmbedder(t *testing.T) *Embedder {
	cacheDir := getTestCacheDir(t)
	return NewEmbedder(cacheDir)
}

func TestEmbedderBasic(t *testing.T) {
	embedder := setupTestEmbedder(t)
	defer embedder.Close()

	if !embedder.IsAvailable() {
		t.Fatal("嵌入器不可用")
	}

	vec, err := embedder.Embed("Hello world")
	if err != nil {
		t.Fatalf("Embed 失败: %v", err)
	}

	if len(vec) != embeddingDim {
		t.Errorf("向量维度 %d, 期望 %d", len(vec), embeddingDim)
	}

	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.01 {
		t.Errorf("L2 范数 %f, 期望约 1.0", norm)
	}
}

func TestEmbedderSimilarity(t *testing.T) {
	embedder := setupTestEmbedder(t)
	defer embedder.Close()

	vec1, _ := embedder.Embed("I am happy today")
	vec2, _ := embedder.Embed("I am joyful today")
	vec3, _ := embedder.Embed("The weather in Beijing is cold")

	sim12 := cosine(vec1, vec2)
	sim13 := cosine(vec1, vec3)

	t.Logf("happy/joyful similarity: %.4f", sim12)
	t.Logf("happy/weather similarity: %.4f", sim13)

	if sim12 <= sim13 {
		t.Errorf("语义相似文本的余弦相似度 (%.4f) 应高于不相关文本 (%.4f)", sim12, sim13)
	}
}

func TestEmbedderMultilingual(t *testing.T) {
	embedder := setupTestEmbedder(t)
	defer embedder.Close()

	vecEN, _ := embedder.Embed("Hello world")
	vecZH, _ := embedder.Embed("你好世界")
	vecFR, _ := embedder.Embed("Bonjour le monde")

	simENZH := cosine(vecEN, vecZH)
	simENFR := cosine(vecEN, vecFR)

	t.Logf("EN/ZH similarity: %.4f", simENZH)
	t.Logf("EN/FR similarity: %.4f", simENFR)

	if simENZH < 0.5 {
		t.Errorf("英中相似度 %.4f 过低, 期望 > 0.5", simENZH)
	}
	if simENFR < 0.5 {
		t.Errorf("英法相似度 %.4f 过低, 期望 > 0.5", simENFR)
	}
}

func cosine(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
