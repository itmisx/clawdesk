package memory

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func getTestCacheDir(t *testing.T) string {
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".clawdesk", "cache", "ort")
	if _, err := os.Stat(filepath.Join(cacheDir, ".ready")); err != nil {
		t.Skip("嵌入资源未就绪，请先运行 make setup-cache")
	}
	return cacheDir
}

func loadTestTokenizer(t *testing.T) *Tokenizer {
	cacheDir := getTestCacheDir(t)
	data, err := os.ReadFile(filepath.Join(cacheDir, "tokenizer.json"))
	if err != nil {
		t.Fatalf("读取 tokenizer.json 失败: %v", err)
	}
	tok, err := NewTokenizer(data)
	if err != nil {
		t.Fatalf("创建 tokenizer 失败: %v", err)
	}
	return tok
}

func TestTokenizerEncode(t *testing.T) {
	tok := loadTestTokenizer(t)

	tests := []struct {
		text    string
		wantIDs []int64
	}{
		{
			text:    "Hello world",
			wantIDs: []int64{0, 35378, 8999, 2},
		},
		{
			text:    "query: how are you?",
			wantIDs: []int64{0, 41, 1294, 12, 3642, 621, 398, 32, 2},
		},
		{
			text:    "你好世界",
			wantIDs: []int64{0, 6, 124084, 3221, 2},
		},
		{
			text:    "Bonjour le monde",
			wantIDs: []int64{0, 84602, 95, 11146, 2},
		},
		{
			text:    "user: The quick brown fox jumps over the lazy dog",
			wantIDs: []int64{0, 38937, 12, 581, 63773, 119455, 6, 147797, 88203, 7, 645, 70, 21, 3285, 10269, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			ids, mask := tok.Encode(tt.text)
			if !reflect.DeepEqual(ids, tt.wantIDs) {
				t.Errorf("Encode(%q)\n  got  IDs: %v\n  want IDs: %v", tt.text, ids, tt.wantIDs)
			}
			if len(mask) != len(ids) {
				t.Errorf("mask length %d != ids length %d", len(mask), len(ids))
			}
			for i, v := range mask {
				if v != 1 {
					t.Errorf("mask[%d] = %d, want 1", i, v)
				}
			}
		})
	}
}
