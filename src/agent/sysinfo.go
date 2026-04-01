package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"clawdesk/src/config"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SystemInfo 系统资源信息
type SystemInfo struct {
	CPUPercent        float64 `json:"cpuPercent"`
	MemUsedMB         uint64  `json:"memUsedMB"`
	MemTotalMB        uint64  `json:"memTotalMB"`
	MemPercent        float64 `json:"memPercent"`
	StorageUsedKB     int64   `json:"storageUsedKB"`
	GoRoutines        int     `json:"goRoutines"`
	EmbeddingReady    bool    `json:"embeddingReady"`
}

// 缓存：低频采集结果，主循环只读缓存
var (
	cachedStorageKB  atomic.Int64
	cachedMemInuse   atomic.Uint64
	cachedMemSys     atomic.Uint64
	cachedCPUPercent atomic.Int64 // 存储 CPU% * 100（用整数模拟两位小数）
)

// GetSystemInfo 获取系统资源信息（只读缓存，不触发 STW）
func (a *App) GetSystemInfo() SystemInfo {
	var info SystemInfo

	info.GoRoutines = runtime.NumGoroutine()
	info.CPUPercent = float64(cachedCPUPercent.Load()) / 100.0
	info.StorageUsedKB = cachedStorageKB.Load()

	memInuse := cachedMemInuse.Load()
	memSys := cachedMemSys.Load()
	info.MemUsedMB = memInuse / 1024 / 1024
	info.MemTotalMB = memSys / 1024 / 1024
	if memSys > 0 {
		info.MemPercent = float64(memInuse) / float64(memSys) * 100
	}

	if a.memMgr != nil {
		info.EmbeddingReady = a.memMgr.IsEmbeddingAvailable()
	}

	return info
}

// rusageTimeMs 将 syscall.Timeval 转为毫秒
func rusageTimeMs(tv syscall.Timeval) int64 {
	return int64(tv.Sec)*1000 + int64(tv.Usec)/1000
}

// sampleProcessCPU 通过 getrusage 采样进程 CPU 时间，计算真实 CPU 使用率
func sampleProcessCPU() {
	numCPU := runtime.NumCPU()
	var ru syscall.Rusage

	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	prevTotal := rusageTimeMs(ru.Utime) + rusageTimeMs(ru.Stime)
	prevWall := time.Now()

	for {
		time.Sleep(5 * time.Second)

		syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
		curTotal := rusageTimeMs(ru.Utime) + rusageTimeMs(ru.Stime)
		curWall := time.Now()

		wallMs := curWall.Sub(prevWall).Milliseconds()
		if wallMs > 0 {
			// CPU% = 进程 CPU 时间差 / 墙钟时间差 / CPU 核数 * 100（0~100%）
			percent := float64(curTotal-prevTotal) / float64(wallMs) / float64(numCPU) * 100
			if percent > 100 {
				percent = 100
			}
			if percent < 0 {
				percent = 0
			}
			cachedCPUPercent.Store(int64(percent * 100))
		}

		prevTotal = curTotal
		prevWall = curWall
	}
}

// dirSizeKB 计算目录大小（KB），排除 cache 子目录
func dirSizeKB(path string) int64 {
	cacheDir := filepath.Join(path, "cache")
	var size int64
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && p == cacheDir {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size / 1024
}

// StartSystemMonitor 启动系统监控，定期推送到前端
func (a *App) StartSystemMonitor() {
	// CPU 采样（独立 goroutine，每 5 秒更新一次真实进程 CPU 使用率）
	go sampleProcessCPU()

	// 低频：每 5 分钟更新目录大小
	go func() {
		for {
			cachedStorageKB.Store(dirSizeKB(config.GetConfigDir()))
			select {
			case <-time.After(5 * time.Minute):
			case <-a.ctx.Done():
				return
			}
		}
	}()

	// 低频：每 30 秒更新内存统计（ReadMemStats 触发 STW，不能放在高频路径）
	go func() {
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			cachedMemInuse.Store(m.HeapInuse)
			cachedMemSys.Store(m.Sys)
			select {
			case <-time.After(30 * time.Second):
			case <-a.ctx.Done():
				return
			}
		}
	}()

	// 主循环：每 5 秒推送系统信息（只读缓存，无 STW）
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if a.ctx == nil {
					return
				}
				wailsRuntime.EventsEmit(a.ctx, "system:info", a.GetSystemInfo())
			case <-a.ctx.Done():
				return
			}
		}
	}()
}
