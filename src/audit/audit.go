package audit

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB 审计数据库（按天分表，自动清理过期数据）
type DB struct {
	db      *sql.DB
	maxDays int // 保留天数
}

// NewDB 创建审计数据库
func NewDB(baseDir string) (*DB, error) {
	auditDir := filepath.Join(baseDir, "audit")
	os.MkdirAll(auditDir, 0755)

	dbPath := filepath.Join(auditDir, "audit.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	db.Exec(`PRAGMA journal_mode=WAL`)
	db.Exec(`PRAGMA synchronous=NORMAL`)
	db.Exec(`PRAGMA cache_size=-4000`)
	db.Exec(`PRAGMA busy_timeout=5000`)

	a := &DB{db: db, maxDays: 30}

	// 清理旧版单表（迁移到按天分表后不再使用）
	db.Exec(`DROP TABLE IF EXISTS skill_audit`)
	db.Exec(`DROP TABLE IF EXISTS storage_audit`)
	db.Exec(`DROP TABLE IF EXISTS schema_version`)

	// 确保今天的表存在
	a.ensureSkillTable(today())
	a.ensureStorageTable(today())

	// 启动时清理过期表
	go a.cleanupOldTables()

	return a, nil
}

func today() string { return time.Now().Format("20060102") }

func (a *DB) skillTable(day string) string   { return "skill_audit_" + day }
func (a *DB) storageTable(day string) string { return "storage_audit_" + day }

func (a *DB) ensureSkillTable(day string) {
	table := a.skillTable(day)
	a.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			bot_name TEXT NOT NULL DEFAULT '',
			skill_name TEXT NOT NULL DEFAULT '',
			tool_name TEXT NOT NULL,
			args TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL DEFAULT '',
			success INTEGER NOT NULL DEFAULT 1,
			duration_ms INTEGER NOT NULL DEFAULT 0
		)
	`, table))
	a.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_ts ON %s(timestamp)`, table, table))
	a.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_tool ON %s(tool_name)`, table, table))
}

func (a *DB) ensureStorageTable(day string) {
	table := a.storageTable(day)
	a.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			type TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			file_name TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			size INTEGER NOT NULL DEFAULT 0,
			count INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			success INTEGER NOT NULL DEFAULT 1,
			error TEXT NOT NULL DEFAULT ''
		)
	`, table))
	a.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_ts ON %s(timestamp)`, table, table))
	a.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_type ON %s(type)`, table, table))
}

// cleanupOldTables 清理过期的按天分表
func (a *DB) cleanupOldTables() {
	cutoff := time.Now().AddDate(0, 0, -a.maxDays).Format("20060102")

	rows, err := a.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND (name LIKE 'skill_audit_%' OR name LIKE 'storage_audit_%')`)
	if err != nil {
		return
	}
	defer rows.Close()

	var toDrop []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		// 提取日期部分
		parts := strings.Split(name, "_")
		if len(parts) < 3 {
			continue
		}
		day := parts[len(parts)-1]
		if len(day) == 8 && day < cutoff {
			toDrop = append(toDrop, name)
		}
	}

	for _, name := range toDrop {
		a.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", name))
	}
}

func (a *DB) Close() error {
	return a.db.Close()
}

// recentDays 返回最近 N 天的日期列表（从新到旧）
func recentDays(days int) []string {
	result := make([]string, 0, days)
	for i := 0; i < days; i++ {
		result = append(result, time.Now().AddDate(0, 0, -i).Format("20060102"))
	}
	return result
}

// ===== 技能审计 =====

// SkillRecord 技能审计记录
type SkillRecord struct {
	ID         int64  `json:"id"`
	Timestamp  string `json:"timestamp"`
	SessionID  string `json:"sessionId"`
	BotName    string `json:"botName"`
	SkillName  string `json:"skillName"`
	ToolName   string `json:"toolName"`
	Args       string `json:"args"`
	Result     string `json:"result"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"durationMs"`
}

// RecordSkill 记录技能调用
func (a *DB) RecordSkill(sessionID, botName, skillName, toolName, args, result string, success bool, durationMs int64) {
	if len(result) > 500 {
		result = result[:500] + "..."
	}
	day := today()
	a.ensureSkillTable(day)
	a.db.Exec(
		fmt.Sprintf(`INSERT INTO %s (timestamp, session_id, bot_name, skill_name, tool_name, args, result, success, duration_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, a.skillTable(day)),
		time.Now().Format(time.RFC3339Nano), sessionID, botName, skillName, toolName, args, result, boolToInt(success), durationMs,
	)
}

// SkillQuery 技能审计查询参数
type SkillQuery struct {
	Days     int    `json:"days"`     // 最近 N 天，0 表示全部
	ToolName string `json:"toolName"` // 按工具名过滤
	Page     int    `json:"page"`     // 页码（从 1 开始）
	PageSize int    `json:"pageSize"` // 每页条数
}

// SkillPageResult 分页结果
type SkillPageResult struct {
	Records []SkillRecord `json:"records"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PageSize int          `json:"pageSize"`
}

// GetSkillRecords 查询技能审计记录（分页 + 过滤）
func (a *DB) GetSkillRecords(query SkillQuery) SkillPageResult {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	days := query.Days
	if days < 1 {
		days = a.maxDays
	}

	dayList := recentDays(days)

	// 构建 UNION ALL 查询
	var unions []string
	for _, day := range dayList {
		table := a.skillTable(day)
		if a.tableExists(table) {
			unions = append(unions, fmt.Sprintf("SELECT id, timestamp, session_id, bot_name, skill_name, tool_name, args, result, success, duration_ms FROM %s", table))
		}
	}
	if len(unions) == 0 {
		return SkillPageResult{Records: []SkillRecord{}, Page: query.Page, PageSize: query.PageSize}
	}

	unionSQL := strings.Join(unions, " UNION ALL ")

	// 构建 WHERE
	where := ""
	var filterArgs []any
	if query.ToolName != "" {
		where = " WHERE tool_name = ?"
		filterArgs = append(filterArgs, query.ToolName)
	}

	// 总数
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s)%s", unionSQL, where)
	a.db.QueryRow(countSQL, filterArgs...).Scan(&total)

	// 分页查询
	offset := (query.Page - 1) * query.PageSize
	dataSQL := fmt.Sprintf("SELECT * FROM (%s)%s ORDER BY timestamp DESC LIMIT ? OFFSET ?", unionSQL, where)
	dataArgs := append(filterArgs, query.PageSize, offset)

	rows, err := a.db.Query(dataSQL, dataArgs...)
	if err != nil {
		return SkillPageResult{Records: []SkillRecord{}, Total: total, Page: query.Page, PageSize: query.PageSize}
	}
	defer rows.Close()

	var records []SkillRecord
	for rows.Next() {
		var r SkillRecord
		var success int
		rows.Scan(&r.ID, &r.Timestamp, &r.SessionID, &r.BotName, &r.SkillName, &r.ToolName, &r.Args, &r.Result, &success, &r.DurationMs)
		r.Success = success != 0
		records = append(records, r)
	}
	if records == nil {
		records = []SkillRecord{}
	}

	return SkillPageResult{Records: records, Total: total, Page: query.Page, PageSize: query.PageSize}
}

// SkillStats 技能审计统计
type SkillStats struct {
	TotalCalls   int            `json:"totalCalls"`
	SuccessCalls int            `json:"successCalls"`
	FailedCalls  int            `json:"failedCalls"`
	ByTool       map[string]int `json:"byTool"`
	ByBot        map[string]int `json:"byBot"`
}

// GetSkillStats 获取技能统计（最近 N 天）
func (a *DB) GetSkillStats(days int) SkillStats {
	if days < 1 {
		days = a.maxDays
	}
	stats := SkillStats{ByTool: make(map[string]int), ByBot: make(map[string]int)}

	dayList := recentDays(days)
	var unions []string
	for _, day := range dayList {
		table := a.skillTable(day)
		if a.tableExists(table) {
			unions = append(unions, fmt.Sprintf("SELECT tool_name, bot_name, success FROM %s", table))
		}
	}
	if len(unions) == 0 {
		return stats
	}
	unionSQL := strings.Join(unions, " UNION ALL ")

	a.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM (%s)", unionSQL)).Scan(&stats.TotalCalls)
	a.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM (%s) WHERE success = 1", unionSQL)).Scan(&stats.SuccessCalls)
	stats.FailedCalls = stats.TotalCalls - stats.SuccessCalls

	rows, _ := a.db.Query(fmt.Sprintf("SELECT tool_name, COUNT(*) FROM (%s) GROUP BY tool_name", unionSQL))
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var count int
			rows.Scan(&name, &count)
			stats.ByTool[name] = count
		}
	}

	rows2, _ := a.db.Query(fmt.Sprintf("SELECT bot_name, COUNT(*) FROM (%s) WHERE bot_name != '' GROUP BY bot_name", unionSQL))
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var name string
			var count int
			rows2.Scan(&name, &count)
			stats.ByBot[name] = count
		}
	}

	return stats
}

// ===== 存储审计 =====

// StorageRecord 存储审计记录
type StorageRecord struct {
	ID         int64  `json:"id"`
	Timestamp  string `json:"timestamp"`
	Type       string `json:"type"`
	SessionID  string `json:"sessionId"`
	FileName   string `json:"fileName"`
	Detail     string `json:"detail"`
	Size       int64  `json:"size"`
	Count      int    `json:"count"`
	DurationMs int64  `json:"durationMs"`
	Success    bool   `json:"success"`
	Error      string `json:"error"`
}

// RecordStorage 记录存储操作
func (a *DB) RecordStorage(typ, sessionID, fileName, detail string, size int64, count int, durationMs int64, success bool, errMsg string) {
	day := today()
	a.ensureStorageTable(day)
	a.db.Exec(
		fmt.Sprintf(`INSERT INTO %s (timestamp, type, session_id, file_name, detail, size, count, duration_ms, success, error) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, a.storageTable(day)),
		time.Now().Format(time.RFC3339Nano), typ, sessionID, fileName, detail, size, count, durationMs, boolToInt(success), errMsg,
	)
}

// StorageQuery 存储审计查询参数
type StorageQuery struct {
	Days     int `json:"days"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// StoragePageResult 分页结果
type StoragePageResult struct {
	Records  []StorageRecord `json:"records"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

// GetStorageRecords 查询存储审计记录（分页）
func (a *DB) GetStorageRecords(query StorageQuery) StoragePageResult {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	days := query.Days
	if days < 1 {
		days = a.maxDays
	}

	dayList := recentDays(days)
	var unions []string
	for _, day := range dayList {
		table := a.storageTable(day)
		if a.tableExists(table) {
			unions = append(unions, fmt.Sprintf("SELECT id, timestamp, type, session_id, file_name, detail, size, count, duration_ms, success, error FROM %s", table))
		}
	}
	if len(unions) == 0 {
		return StoragePageResult{Records: []StorageRecord{}, Page: query.Page, PageSize: query.PageSize}
	}

	unionSQL := strings.Join(unions, " UNION ALL ")

	var total int
	a.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM (%s)", unionSQL)).Scan(&total)

	offset := (query.Page - 1) * query.PageSize
	rows, err := a.db.Query(fmt.Sprintf("SELECT * FROM (%s) ORDER BY timestamp DESC LIMIT ? OFFSET ?", unionSQL), query.PageSize, offset)
	if err != nil {
		return StoragePageResult{Records: []StorageRecord{}, Total: total, Page: query.Page, PageSize: query.PageSize}
	}
	defer rows.Close()

	var records []StorageRecord
	for rows.Next() {
		var r StorageRecord
		var success int
		rows.Scan(&r.ID, &r.Timestamp, &r.Type, &r.SessionID, &r.FileName, &r.Detail, &r.Size, &r.Count, &r.DurationMs, &success, &r.Error)
		r.Success = success != 0
		records = append(records, r)
	}
	if records == nil {
		records = []StorageRecord{}
	}

	return StoragePageResult{Records: records, Total: total, Page: query.Page, PageSize: query.PageSize}
}

// StorageStats 存储审计统计
type StorageStats struct {
	TotalOps   int            `json:"totalOps"`
	SuccessOps int            `json:"successOps"`
	FailedOps  int            `json:"failedOps"`
	TotalBytes int64          `json:"totalBytes"`
	ByType     map[string]int `json:"byType"`
}

// GetStorageStats 获取存储统计（最近 N 天）
func (a *DB) GetStorageStats(days int) StorageStats {
	if days < 1 {
		days = a.maxDays
	}
	stats := StorageStats{ByType: make(map[string]int)}

	dayList := recentDays(days)
	var unions []string
	for _, day := range dayList {
		table := a.storageTable(day)
		if a.tableExists(table) {
			unions = append(unions, fmt.Sprintf("SELECT type, size, success FROM %s", table))
		}
	}
	if len(unions) == 0 {
		return stats
	}
	unionSQL := strings.Join(unions, " UNION ALL ")

	a.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM (%s)", unionSQL)).Scan(&stats.TotalOps)
	a.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM (%s) WHERE success = 1", unionSQL)).Scan(&stats.SuccessOps)
	stats.FailedOps = stats.TotalOps - stats.SuccessOps
	a.db.QueryRow(fmt.Sprintf("SELECT COALESCE(SUM(size), 0) FROM (%s)", unionSQL)).Scan(&stats.TotalBytes)

	rows, _ := a.db.Query(fmt.Sprintf("SELECT type, COUNT(*) FROM (%s) GROUP BY type", unionSQL))
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var typ string
			var count int
			rows.Scan(&typ, &count)
			stats.ByType[typ] = count
		}
	}

	return stats
}

// tableExists 检查表是否存在
func (a *DB) tableExists(name string) bool {
	var count int
	a.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count)
	return count > 0
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
