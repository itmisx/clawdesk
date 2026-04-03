package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"clawdesk/src/config"
)

// Config 记忆系统配置
type Config struct {
	RecentKeep          int     // 保留最近消息数上限（默认 10）
	RecentTokenBudget   int     // 近期消息 token 预算（默认 4000，用 rune 数估算）
	VectorTopK          int     // 向量搜索返回数（默认 5）
	CompressThreshold   int     // 触发压缩的消息阈值（默认 50）
	SimilarityThreshold float64 // 向量检索最低相似度（默认 0.5）
	TimeDecayRate       float64 // 时间衰减系数（每天衰减比例，默认 0.005）
}

func DefaultConfig() Config {
	return Config{
		RecentKeep:          10,
		RecentTokenBudget:   4000,
		VectorTopK:          5,
		CompressThreshold:   50,
		SimilarityThreshold: 0.5,
		TimeDecayRate:       0.005,
	}
}

// StorageAuditor 存储审计接口（避免循环依赖）
type StorageAuditor interface {
	RecordStorage(typ, sessionID, fileName, detail string, size int64, count int, durationMs int64, success bool, errMsg string)
}

// MemoryManager 记忆管理器
type MemoryManager struct {
	Store        *DailyStore
	vectorDB     *VectorDB
	embedder     *Embedder
	compressor   *Compressor
	config       Config
	Auditor      StorageAuditor
}

func NewMemoryManager(baseDir string, llmCall LLMCallFunc) (*MemoryManager, error) {
	cfg := DefaultConfig()

	store := NewDailyStore(baseDir)
	store.MigrateFromLegacy()

	dbPath := filepath.Join(baseDir, "vectors.db")
	vectorDB, err := NewVectorDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("初始化向量数据库失败: %w", err)
	}

	// 嵌入资源将在后台异步下载到缓存目录
	ortCacheDir := filepath.Join(getCacheDir(), "ort")
	embedder := NewEmbedder(ortCacheDir)
	compressor := NewCompressor(store, cfg.CompressThreshold, llmCall)

	return &MemoryManager{
		Store:      store,
		vectorDB:   vectorDB,
		embedder:   embedder,
		compressor: compressor,
		config:     cfg,
	}, nil
}

// StartAssetDownload 在后台启动嵌入资源下载
func (mm *MemoryManager) StartAssetDownload(onProgress DownloadProgressFunc) {
	go func() {
		if err := DownloadAssetsIfNeeded(mm.embedder.cacheDir, onProgress); err != nil {
			fmt.Printf("下载嵌入资源失败（非致命）: %v\n", err)
		}
	}()
}

func (mm *MemoryManager) Close() error {
	mm.embedder.Close()
	return mm.vectorDB.Close()
}

// StoreMessage 存储消息并异步向量化
func (mm *MemoryManager) StoreMessage(sessionID string, msg StoredMessage) error {
	fileName := msg.Timestamp.Format("20060102") + ".jsonl"
	start := time.Now()

	err := mm.Store.AppendMessage(sessionID, msg)
	mm.logStorage("write_message", sessionID, fileName,
		fmt.Sprintf("[%s] %s", msg.Role, truncate(msg.Content, 80)),
		int64(len(msg.Content)), 0, time.Since(start).Milliseconds(), err == nil, errStr(err))
	if err != nil {
		return err
	}

	if mm.embedder.IsAvailable() && (msg.Role == "user" || msg.Role == "assistant") {
		go func() {
			vStart := time.Now()
			emb, err := mm.embedder.Embed(msg.Role + ": " + msg.Content)
			if err != nil {
				mm.logStorage("write_vector", sessionID, fileName,
					"embedding failed", 0, 0, time.Since(vStart).Milliseconds(), false, err.Error())
				return
			}
			vErr := mm.vectorDB.Store(sessionID, fileName, msg.Timestamp, msg.Role, emb)
			mm.logStorage("write_vector", sessionID, fileName,
				fmt.Sprintf("[%s] dim=%d", msg.Role, len(emb)),
				int64(len(emb)*4), 0, time.Since(vStart).Milliseconds(), vErr == nil, errStr(vErr))
		}()
	}

	return nil
}

// BuildContext 构建 LLM 请求的上下文消息列表
//
// 最终结构:
//
//	[0]    system:  系统提示词 + 压缩摘要（如有）
//	[1]    system:  向量检索的相关历史片段（如有）
//	[2..N] user/assistant: 最近 N 条消息（不含当前输入）
//
// 注意: 不包含当前用户输入，由调用方追加
func (mm *MemoryManager) BuildContext(sessionID string, systemPrompt string, userInput string) ([]StoredMessage, error) {
	ctxStart := time.Now()
	var messages []StoredMessage

	// === 1. 系统提示词 + 压缩摘要 ===
	enhancedPrompt := systemPrompt
	summary, summaryErr := mm.compressor.LoadSummary(sessionID)
	if summaryErr == nil && summary != nil && summary.Summary != "" {
		enhancedPrompt += "\n\n--- 历史对话摘要 ---\n" + summary.Summary
		mm.logStorage("load_summary", sessionID, "", fmt.Sprintf("loaded summary, %d chars", len(summary.Summary)), 0, 0, 0, true, "")
	}

	messages = append(messages, StoredMessage{
		Timestamp: time.Now(),
		Role:      "system",
		Content:   enhancedPrompt,
	})

	// === 2. 加载最近消息（自适应 token 预算） ===
	loadStart := time.Now()
	// 多加载一些消息用于 token 预算裁剪（+1 用于排除当前输入）
	recentMsgs, _ := mm.Store.LoadRecentMessages(sessionID, mm.config.RecentKeep+1)
	mm.logStorage("load_messages", sessionID, "", fmt.Sprintf("loaded %d recent messages", len(recentMsgs)), 0, len(recentMsgs), time.Since(loadStart).Milliseconds(), true, "")

	// 排除最后一条（即当前用户刚输入的消息，由调用方单独追加）
	if len(recentMsgs) > 0 {
		last := recentMsgs[len(recentMsgs)-1]
		if last.Role == "user" && last.Content == userInput {
			recentMsgs = recentMsgs[:len(recentMsgs)-1]
		}
	}

	// 基于 token 预算裁剪：从最新消息向前累加 rune 数，超出预算时截断
	if mm.config.RecentTokenBudget > 0 && len(recentMsgs) > 0 {
		runeCount := 0
		cutoff := 0
		for i := len(recentMsgs) - 1; i >= 0; i-- {
			msgRunes := len([]rune(fmt.Sprintf("%v", recentMsgs[i].Content)))
			if runeCount+msgRunes > mm.config.RecentTokenBudget && i < len(recentMsgs)-1 {
				cutoff = i + 1
				break
			}
			runeCount += msgRunes
		}
		if cutoff > 0 {
			recentMsgs = recentMsgs[cutoff:]
		}
	}

	// === 3. 向量搜索相关历史（与最近消息去重） ===
	if mm.embedder.IsAvailable() && userInput != "" {
		searchStart := time.Now()
		queryEmb, err := mm.embedder.Embed("user: " + userInput)
		if err == nil {
			results, err := mm.vectorDB.Search(sessionID, queryEmb, mm.config.VectorTopK+len(recentMsgs))
			if err == nil && len(results) > 0 {
				// 建立最近消息时间戳集合用于去重
				recentTSSet := make(map[string]bool)
				for _, m := range recentMsgs {
					recentTSSet[m.Timestamp.Format(time.RFC3339Nano)] = true
				}

				// 应用时间衰减权重并过滤
				type scoredResult struct {
					SearchResult
					adjustedScore float64
				}
				var filtered []scoredResult
				now := time.Now()
				for _, r := range results {
					if recentTSSet[r.MessageTS.Format(time.RFC3339Nano)] {
						continue // 跳过已在最近消息中的
					}
					if r.Similarity < mm.config.SimilarityThreshold {
						continue // 相似度过低
					}
					// 时间衰减：近期消息轻微加权
					daysSince := now.Sub(r.MessageTS).Hours() / 24
					decay := 1.0 - mm.config.TimeDecayRate*daysSince
					if decay < 0.1 {
						decay = 0.1 // 最低保留 10% 权重
					}
					filtered = append(filtered, scoredResult{r, r.Similarity * decay})
				}
				// 按调整后分数重新排序
				sort.Slice(filtered, func(i, j int) bool {
					return filtered[i].adjustedScore > filtered[j].adjustedScore
				})
				if len(filtered) > mm.config.VectorTopK {
					filtered = filtered[:mm.config.VectorTopK]
				}

				if len(filtered) > 0 {
					var contextText strings.Builder
					contextText.WriteString("以下是与当前问题相关的历史对话片段（仅供参考）：\n")
					for _, r := range filtered {
						date := r.MessageTS.Format("2006-01-02 15:04")
						// 通过文件名回读原文
						content := mm.loadMessageContent(sessionID, r.FileName, r.MessageTS)
						if content == "" {
							continue
						}
						contextText.WriteString(fmt.Sprintf("[%s][%s] %s\n", date, r.Role, truncate(content, 300)))
					}

					messages = append(messages, StoredMessage{
						Timestamp: time.Now(),
						Role:      "system",
						Content:   contextText.String(),
					})
				}
			}
			mm.logStorage("search_vector", sessionID, "", fmt.Sprintf("searched %d candidates", len(results)), 0, len(results), time.Since(searchStart).Milliseconds(), true, "")
		}
	}

	// === 4. 追加最近消息 ===
	messages = append(messages, recentMsgs...)

	mm.logStorage("build_context", sessionID, "", fmt.Sprintf("context built: %d messages total", len(messages)), 0, len(messages), time.Since(ctxStart).Milliseconds(), true, "")

	return messages, nil
}

// TriggerCompressionIfNeeded 异步检查并执行压缩
func (mm *MemoryManager) TriggerCompressionIfNeeded(ctx context.Context, sessionID string) {
	go func() {
		if mm.compressor.ShouldCompress(sessionID, mm.config.RecentKeep) {
			mm.compressor.Compress(ctx, sessionID, mm.config.RecentKeep)
		}
	}()
}

func (mm *MemoryManager) DeleteSession(sessionID string) error {
	mm.Store.DeleteSession(sessionID)
	mm.vectorDB.DeleteSession(sessionID)
	mm.logStorage("delete_session", sessionID, "", "session deleted (files + vectors)", 0, 0, 0, true, "")
	return nil
}

func (mm *MemoryManager) IsEmbeddingAvailable() bool {
	return mm.embedder.IsAvailable()
}

func (mm *MemoryManager) GetEmbeddingModelPath() string {
	return mm.embedder.ModelPath()
}

// getCacheDir 获取缓存目录（~/.clawdesk/cache/）
func getCacheDir() string {
	cacheDir := filepath.Join(config.GetConfigDir(), "cache")
	os.MkdirAll(cacheDir, 0755)
	return cacheDir
}

func (mm *MemoryManager) logStorage(typ, sessionID, fileName, detail string, size int64, count int, durationMs int64, success bool, errMsg string) {
	if mm.Auditor != nil {
		mm.Auditor.RecordStorage(typ, sessionID, fileName, detail, size, count, durationMs, success, errMsg)
	}
}

func errStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

// loadMessageContent 通过文件名和时间戳从 JSONL 回读消息原文
func (mm *MemoryManager) loadMessageContent(sessionID, fileName string, ts time.Time) string {
	dir := mm.Store.sessionDir(sessionID)
	msgs, err := mm.Store.readJSONL(dir + "/" + fileName)
	if err != nil {
		return ""
	}
	// 按时间戳精确匹配
	tsStr := ts.Format(time.RFC3339Nano)
	for _, m := range msgs {
		if m.Timestamp.Format(time.RFC3339Nano) == tsStr {
			return m.Content
		}
	}
	return ""
}
