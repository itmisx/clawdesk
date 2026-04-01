package memory

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ORT 下载 URL 由各平台文件（ort_*.go）的 ortDownloadURL 常量提供
	modelURL     = "https://huggingface.co/xenova/multilingual-e5-small/resolve/main/onnx/model_quantized.onnx"
	tokenizerURL = "https://huggingface.co/intfloat/multilingual-e5-small/resolve/main/tokenizer.json"

	modelFileName     = "multilingual-e5-small-quantized.onnx"
	tokenizerFileName = "tokenizer.json"
	readyMarker       = ".ready"
)

// DownloadProgressFunc 下载进度回调
type DownloadProgressFunc func(fileName string, current, total int64)

// DownloadAssetsIfNeeded 检查并下载所有嵌入资源到 cacheDir
func DownloadAssetsIfNeeded(cacheDir string, onProgress DownloadProgressFunc) error {
	if isReady(cacheDir) {
		return nil
	}

	os.MkdirAll(cacheDir, 0755)

	// 1. 下载 ONNX 模型
	modelPath := filepath.Join(cacheDir, modelFileName)
	if !fileExists(modelPath) {
		if err := downloadFile(modelURL, modelPath, modelFileName, onProgress); err != nil {
			return fmt.Errorf("下载 ONNX 模型失败: %w", err)
		}
	}

	// 2. 下载 tokenizer.json
	tokenizerPath := filepath.Join(cacheDir, tokenizerFileName)
	if !fileExists(tokenizerPath) {
		if err := downloadFile(tokenizerURL, tokenizerPath, tokenizerFileName, onProgress); err != nil {
			return fmt.Errorf("下载 tokenizer 失败: %w", err)
		}
	}

	// 3. 下载并解压 ONNX Runtime 库
	libPath := filepath.Join(cacheDir, ortLibName)
	if !fileExists(libPath) {
		if err := downloadAndExtractORT(cacheDir, onProgress); err != nil {
			return fmt.Errorf("下载 ONNX Runtime 失败: %w", err)
		}
	}

	// 写入就绪标记
	os.WriteFile(filepath.Join(cacheDir, readyMarker), []byte("ok"), 0644)
	fmt.Printf("嵌入资源已就绪: %s\n", cacheDir)
	return nil
}

// isReady 检查资源是否已就绪
func isReady(cacheDir string) bool {
	_, err := os.Stat(filepath.Join(cacheDir, readyMarker))
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// downloadFile 带进度回调的 HTTP 下载
func downloadFile(url, destPath, displayName string, onProgress DownloadProgressFunc) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	tmpPath := destPath + ".tmp"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	var reader io.Reader = resp.Body
	if onProgress != nil {
		reader = &progressReader{
			reader:   resp.Body,
			total:    resp.ContentLength,
			name:     displayName,
			callback: onProgress,
		}
	}

	_, err = io.Copy(dst, reader)
	dst.Close()
	if err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, destPath)
}

// downloadAndExtractORT 下载 ORT 归档并提取库文件
func downloadAndExtractORT(cacheDir string, onProgress DownloadProgressFunc) error {
	archiveURL := ortDownloadURL

	// 下载归档到临时文件
	tmpFile, err := os.CreateTemp("", "ort-*."+ortArchiveFormat)
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := downloadFile(archiveURL, tmpPath, ortLibName, onProgress); err != nil {
		return err
	}

	// 解压提取目标文件
	destPath := filepath.Join(cacheDir, ortLibName)

	switch ortArchiveFormat {
	case "tgz":
		if err := extractFromTgz(tmpPath, ortArchiveLibPath, destPath); err != nil {
			return err
		}
	case "zip":
		if err := extractFromZip(tmpPath, ortArchiveLibPath, destPath); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持的归档格式: %s", ortArchiveFormat)
	}

	os.Chmod(destPath, 0755)
	return nil
}

// extractFromTgz 从 .tgz 归档中提取指定文件
func extractFromTgz(archivePath, targetPath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Name == targetPath || strings.HasSuffix(header.Name, "/"+filepath.Base(targetPath)) {
			dst, err := os.Create(destPath)
			if err != nil {
				return err
			}
			_, err = io.Copy(dst, tr)
			dst.Close()
			return err
		}
	}
	return fmt.Errorf("归档中未找到: %s", targetPath)
}

// extractFromZip 从 .zip 归档中提取指定文件
func extractFromZip(archivePath, targetPath, destPath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == targetPath || strings.HasSuffix(f.Name, "/"+filepath.Base(targetPath)) {
			src, err := f.Open()
			if err != nil {
				return err
			}
			defer src.Close()

			dst, err := os.Create(destPath)
			if err != nil {
				return err
			}
			_, err = io.Copy(dst, src)
			dst.Close()
			return err
		}
	}
	return fmt.Errorf("归档中未找到: %s", targetPath)
}

// progressReader 包装 io.Reader 报告读取进度
type progressReader struct {
	reader   io.Reader
	total    int64
	current  int64
	name     string
	callback DownloadProgressFunc
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.current += int64(n)
	pr.callback(pr.name, pr.current, pr.total)
	return n, err
}
