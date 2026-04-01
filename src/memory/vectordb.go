package memory

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

const schemaVersion = 4

// SearchResult 向量搜索结果
type SearchResult struct {
	SessionID  string
	FileName   string
	MessageTS  time.Time
	Role       string
	Similarity float64
}

// VectorDB SQLite 向量数据库
type VectorDB struct {
	db *sql.DB
}

func NewVectorDB(dbPath string) (*VectorDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	// SQLite 性能优化
	db.Exec(`PRAGMA journal_mode=WAL`)
	db.Exec(`PRAGMA synchronous=NORMAL`)
	db.Exec(`PRAGMA cache_size=-8000`) // 8MB cache
	db.Exec(`PRAGMA busy_timeout=5000`)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	return &VectorDB{db: db}, nil
}

// sessionTable 返回会话的分表名
func sessionTable(sessionID string) string {
	return "emb_" + sessionID
}

// migrate 数据库版本迁移
func migrate(db *sql.DB) error {
	// schema_version 表
	db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)

	var ver int
	err := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&ver)
	if err != nil {
		ver = 0
		db.Exec(`INSERT INTO schema_version (version) VALUES (0)`)
	}

	if ver < 1 {
		// v1: 初始表结构
		db.Exec(`DROP TABLE IF EXISTS embeddings`)
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS embeddings (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				session_id TEXT NOT NULL,
				file_name TEXT NOT NULL,
				message_ts TEXT NOT NULL,
				role TEXT NOT NULL,
				dimension INTEGER NOT NULL DEFAULT 0,
				embedding BLOB NOT NULL,
				UNIQUE(session_id, message_ts)
			)
		`)
		if err != nil {
			return err
		}
		ver = 1
	}

	if ver < 2 {
		// v2: 索引优化 + 统计表
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_emb_session ON embeddings(session_id)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_emb_file ON embeddings(session_id, file_name)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_emb_role ON embeddings(session_id, role)`)

		db.Exec(`
			CREATE TABLE IF NOT EXISTS embedding_stats (
				session_id TEXT PRIMARY KEY,
				total_count INTEGER DEFAULT 0,
				last_updated TEXT
			)
		`)
		ver = 2
	}

	if ver < 3 {
		// v3: 按会话分表存储，迁移旧数据
		migrateToSessionTables(db)
		ver = 3
	}

	if ver < 4 {
		// v4: 嵌入模型从 GGUF (llama.cpp) 迁移到 ONNX (onnxruntime)，向量不兼容，清空所有嵌入分表
		dropAllEmbeddingTables(db)
		ver = 4
	}

	// 更新版本号
	db.Exec(`UPDATE schema_version SET version = ?`, ver)
	return nil
}

// migrateToSessionTables 将 embeddings 单表数据迁移到 per-session 分表
func migrateToSessionTables(db *sql.DB) {
	// 获取所有 session_id
	rows, err := db.Query(`SELECT DISTINCT session_id FROM embeddings`)
	if err != nil {
		return
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var sid string
		if rows.Scan(&sid) == nil {
			sessionIDs = append(sessionIDs, sid)
		}
	}

	for _, sid := range sessionIDs {
		tbl := sessionTable(sid)
		// 创建分表
		db.Exec(fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS "%s" (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				file_name TEXT NOT NULL,
				message_ts TEXT NOT NULL UNIQUE,
				role TEXT NOT NULL,
				dimension INTEGER NOT NULL DEFAULT 0,
				embedding BLOB NOT NULL
			)
		`, tbl))

		// 迁移数据
		db.Exec(fmt.Sprintf(`
			INSERT OR IGNORE INTO "%s" (file_name, message_ts, role, dimension, embedding)
			SELECT file_name, message_ts, role, dimension, embedding FROM embeddings WHERE session_id = ?
		`, tbl), sid)
	}

	// 删除旧表和索引
	db.Exec(`DROP TABLE IF EXISTS embeddings`)
	db.Exec(`DROP TABLE IF EXISTS embedding_stats`)
}

// dropAllEmbeddingTables 删除所有 emb_* 分表（模型格式变更时清空旧向量）
func dropAllEmbeddingTables(db *sql.DB) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'emb_%'`)
	if err != nil {
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			tables = append(tables, name)
		}
	}
	for _, tbl := range tables {
		db.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, tbl))
	}
}

func (v *VectorDB) Close() error {
	return v.db.Close()
}

// ensureTable 确保会话分表存在
func (v *VectorDB) ensureTable(sessionID string) {
	tbl := sessionTable(sessionID)
	v.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s" (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_name TEXT NOT NULL,
			message_ts TEXT NOT NULL UNIQUE,
			role TEXT NOT NULL,
			dimension INTEGER NOT NULL DEFAULT 0,
			embedding BLOB NOT NULL
		)
	`, tbl))
}

// Store 存储嵌入向量
func (v *VectorDB) Store(sessionID, fileName string, messageTS time.Time, role string, embedding []float32) error {
	v.ensureTable(sessionID)
	blob := encodeEmbedding(embedding)
	tbl := sessionTable(sessionID)

	_, err := v.db.Exec(
		fmt.Sprintf(`INSERT OR REPLACE INTO "%s" (file_name, message_ts, role, dimension, embedding) VALUES (?, ?, ?, ?, ?)`, tbl),
		fileName, messageTS.Format(time.RFC3339Nano), role, len(embedding), blob,
	)
	return err
}

// Search 搜索最相似的 topK 条消息
func (v *VectorDB) Search(sessionID string, queryEmbedding []float32, topK int) ([]SearchResult, error) {
	tbl := sessionTable(sessionID)

	rows, err := v.db.Query(
		fmt.Sprintf(`SELECT file_name, message_ts, role, embedding FROM "%s"`, tbl),
	)
	if err != nil {
		// 表不存在时返回空结果
		return nil, nil
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var fileName, tsStr, role string
		var blob []byte

		if err := rows.Scan(&fileName, &tsStr, &role, &blob); err != nil {
			continue
		}

		emb := decodeEmbedding(blob)
		if len(emb) != len(queryEmbedding) {
			continue
		}

		sim := cosineSimilarity(queryEmbedding, emb)

		ts, _ := time.Parse(time.RFC3339Nano, tsStr)
		results = append(results, SearchResult{
			SessionID:  sessionID,
			FileName:   fileName,
			MessageTS:  ts,
			Role:       role,
			Similarity: sim,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// DeleteSession 删除会话的所有向量（直接 DROP 分表）
func (v *VectorDB) DeleteSession(sessionID string) error {
	tbl := sessionTable(sessionID)
	_, err := v.db.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, tbl))
	return err
}

// DeleteByFile 删除指定文件的向量
func (v *VectorDB) DeleteByFile(sessionID, fileName string) error {
	tbl := sessionTable(sessionID)
	_, err := v.db.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE file_name = ?`, tbl), fileName)
	return err
}

// Vacuum 压缩数据库释放空间
func (v *VectorDB) Vacuum() error {
	_, err := v.db.Exec(`VACUUM`)
	return err
}

// ===== 辅助函数 =====

func encodeEmbedding(emb []float32) []byte {
	buf := make([]byte, len(emb)*4)
	for i, v := range emb {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func decodeEmbedding(data []byte) []float32 {
	n := len(data) / 4
	emb := make([]float32, n)
	for i := 0; i < n; i++ {
		emb[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return emb
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
