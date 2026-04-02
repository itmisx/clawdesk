package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StoredMessage 持久化消息（每行 JSONL）
type StoredMessage struct {
	Timestamp time.Time `json:"ts"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
}

// SessionMeta 会话/助手元数据
type SessionMeta struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Avatar       string    `json:"avatar"`       // emoji 头像
	Description  string    `json:"description"`  // 助手简介
	SystemPrompt string    `json:"systemPrompt"`
	ProviderID   string    `json:"providerId"`   // 绑定模型厂商（空=全局）
	Model        string    `json:"model"`         // 绑定模型名（空=全局）
	CreatedAt    time.Time `json:"createdAt"`
}

// DailyStore 按日文件存储
type DailyStore struct {
	baseDir string // ~/.clawdesk/sessions
}

func NewDailyStore(baseDir string) *DailyStore {
	dir := filepath.Join(baseDir, "sessions")
	os.MkdirAll(dir, 0755)
	return &DailyStore{baseDir: dir}
}

func (ds *DailyStore) sessionDir(sessionID string) string {
	return filepath.Join(ds.baseDir, sessionID)
}

func (ds *DailyStore) ensureSessionDir(sessionID string) error {
	return os.MkdirAll(ds.sessionDir(sessionID), 0755)
}

// ===== 元数据 =====

func (ds *DailyStore) LoadMeta(sessionID string) (*SessionMeta, error) {
	path := filepath.Join(ds.sessionDir(sessionID), "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (ds *DailyStore) SaveMeta(meta *SessionMeta) error {
	ds.ensureSessionDir(meta.ID)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ds.sessionDir(meta.ID), "meta.json"), data, 0644)
}

func (ds *DailyStore) DeleteSession(sessionID string) error {
	return os.RemoveAll(ds.sessionDir(sessionID))
}

func (ds *DailyStore) ListSessions() ([]SessionMeta, error) {
	entries, err := os.ReadDir(ds.baseDir)
	if err != nil {
		return nil, err
	}

	var sessions []SessionMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := ds.LoadMeta(entry.Name())
		if err != nil {
			continue
		}
		sessions = append(sessions, *meta)
	}
	return sessions, nil
}

// ===== 排序 =====

func (ds *DailyStore) orderFile() string {
	return filepath.Join(ds.baseDir, "order.json")
}

// SaveOrder 保存助手排序
func (ds *DailyStore) SaveOrder(ids []string) error {
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return os.WriteFile(ds.orderFile(), data, 0644)
}

// LoadOrder 加载助手排序，文件不存在返回 nil
func (ds *DailyStore) LoadOrder() []string {
	data, err := os.ReadFile(ds.orderFile())
	if err != nil {
		return nil
	}
	var ids []string
	if json.Unmarshal(data, &ids) != nil {
		return nil
	}
	return ids
}

// ===== 消息 JSONL =====

func (ds *DailyStore) dayFileName(t time.Time) string {
	return t.Format("20060102") + ".jsonl"
}

// AppendMessage 追加消息到当日 JSONL 文件
func (ds *DailyStore) AppendMessage(sessionID string, msg StoredMessage) error {
	ds.ensureSessionDir(sessionID)
	path := filepath.Join(ds.sessionDir(sessionID), ds.dayFileName(msg.Timestamp))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// LoadRecentMessages 从最新文件倒序读取最近 n 条消息
func (ds *DailyStore) LoadRecentMessages(sessionID string, n int) ([]StoredMessage, error) {
	files, err := ds.listJSONLFiles(sessionID)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	// 从最新文件开始倒序读
	var collected []StoredMessage
	for i := len(files) - 1; i >= 0 && len(collected) < n; i-- {
		msgs, err := ds.readJSONL(filepath.Join(ds.sessionDir(sessionID), files[i]))
		if err != nil {
			continue
		}
		collected = append(msgs, collected...) // 头部插入，保持时间顺序
	}

	// 只取最后 n 条
	if len(collected) > n {
		collected = collected[len(collected)-n:]
	}
	return collected, nil
}

// LoadAllMessages 加载全部消息（用于压缩）
func (ds *DailyStore) LoadAllMessages(sessionID string) ([]StoredMessage, error) {
	files, err := ds.listJSONLFiles(sessionID)
	if err != nil {
		return nil, err
	}

	var all []StoredMessage
	for _, f := range files {
		msgs, err := ds.readJSONL(filepath.Join(ds.sessionDir(sessionID), f))
		if err != nil {
			continue
		}
		all = append(all, msgs...)
	}
	return all, nil
}

// CountMessages 统计消息总数
func (ds *DailyStore) CountMessages(sessionID string) (int, error) {
	files, err := ds.listJSONLFiles(sessionID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, f := range files {
		path := filepath.Join(ds.sessionDir(sessionID), f)
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) != "" {
				count++
			}
		}
		file.Close()
	}
	return count, nil
}

// listJSONLFiles 列出按文件名排序的 JSONL 文件
func (ds *DailyStore) listJSONLFiles(sessionID string) ([]string, error) {
	entries, err := os.ReadDir(ds.sessionDir(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func (ds *DailyStore) readJSONL(path string) ([]StoredMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var msgs []StoredMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024) // 256KB per line
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg StoredMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs, scanner.Err()
}

// ===== 请求日志 =====

const maxRequestLogs = 20

// LLMRequestLog 记录一次 LLM 请求的完整信息
type LLMRequestLog struct {
	Timestamp    time.Time          `json:"ts"`
	SystemPrompt string             `json:"systemPrompt"`
	Tools        []map[string]any   `json:"tools,omitempty"`
}

// SaveRequestLog 保存请求日志，仅保留最近 20 条
func (ds *DailyStore) SaveRequestLog(sessionID string, log LLMRequestLog) error {
	ds.ensureSessionDir(sessionID)
	path := filepath.Join(ds.sessionDir(sessionID), "request_logs.json")

	var records []LLMRequestLog
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &records)
	}

	log.Timestamp = time.Now()
	records = append(records, log)
	if len(records) > maxRequestLogs {
		records = records[len(records)-maxRequestLogs:]
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadRequestLogs 读取请求日志
func (ds *DailyStore) LoadRequestLogs(sessionID string) ([]LLMRequestLog, error) {
	path := filepath.Join(ds.sessionDir(sessionID), "request_logs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []LLMRequestLog
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// ===== 迁移旧格式 =====

// legacySession 旧格式会话结构
type legacySession struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SystemPrompt string    `json:"systemPrompt"`
	CreatedAt    time.Time `json:"createdAt"`
	History      []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"history"`
}

// MigrateFromLegacy 迁移旧格式 sessions/{id}.json → sessions/{id}/
func (ds *DailyStore) MigrateFromLegacy() error {
	entries, err := os.ReadDir(ds.baseDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".migrated") {
			continue
		}

		path := filepath.Join(ds.baseDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var old legacySession
		if err := json.Unmarshal(data, &old); err != nil {
			continue
		}

		// 写入新格式
		meta := &SessionMeta{
			ID:           old.ID,
			Name:         old.Name,
			SystemPrompt: old.SystemPrompt,
			CreatedAt:    old.CreatedAt,
		}
		if err := ds.SaveMeta(meta); err != nil {
			fmt.Printf("迁移会话 %s 失败: %v\n", old.ID, err)
			continue
		}

		// 旧消息全部写入创建日期的文件
		baseTime := old.CreatedAt
		for i, msg := range old.History {
			stored := StoredMessage{
				Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
				Role:      msg.Role,
				Content:   msg.Content,
			}
			ds.AppendMessage(old.ID, stored)
		}

		// 重命名旧文件
		os.Rename(path, path+".migrated")
		fmt.Printf("已迁移会话: %s (%s)\n", old.Name, old.ID)
	}
	return nil
}
