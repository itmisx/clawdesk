package agent

import "time"

// FileInfo 文件信息（供前端直接调用的接口使用）
type FileInfo struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}
