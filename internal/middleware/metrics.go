package middleware

import (
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// MemoryStats 内存统计
type MemoryStats struct {
	Alloc        uint64 `json:"alloc"`          // 当前分配的内存 (字节)
	TotalAlloc   uint64 `json:"total_alloc"`    // 累计分配的内存
	Sys          uint64 `json:"sys"`            // 从系统获取的内存
	NumGC        uint32 `json:"num_gc"`         // GC 次数
	Goroutines   int    `json:"goroutines"`     // Goroutine 数量
	AllocMB      uint64 `json:"alloc_mb"`       // 当前分配 (MB)
	SysMB        uint64 `json:"sys_mb"`         // 系统内存 (MB)
}

// MemoryMonitor 内存监控器
type MemoryMonitor struct {
	logger     *logrus.Logger
	stats      *MemoryStats
	mutex      sync.RWMutex
	stopChan   chan struct{}
	interval   time.Duration
}

// NewMemoryMonitor 创建内存监控器
func NewMemoryMonitor(logger *logrus.Logger, interval time.Duration) *MemoryMonitor {
	return &MemoryMonitor{
		logger:   logger,
		stats:    &MemoryStats{},
		stopChan: make(chan struct{}),
		interval: interval,
	}
}

// Start 启动内存监控
func (m *MemoryMonitor) Start() {
	go m.monitor()
}

// Stop 停止内存监控
func (m *MemoryMonitor) Stop() {
	close(m.stopChan)
}

// monitor 监控循环
func (m *MemoryMonitor) monitor() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.updateStats()
			m.logStats()
		}
	}
}

// updateStats 更新统计信息
func (m *MemoryMonitor) updateStats() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.stats.Alloc = ms.Alloc
	m.stats.TotalAlloc = ms.TotalAlloc
	m.stats.Sys = ms.Sys
	m.stats.NumGC = ms.NumGC
	m.stats.Goroutines = runtime.NumGoroutine()
	m.stats.AllocMB = ms.Alloc / 1024 / 1024
	m.stats.SysMB = ms.Sys / 1024 / 1024
}

// logStats 记录统计信息
func (m *MemoryMonitor) logStats() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.logger.WithFields(logrus.Fields{
		"alloc_mb":    m.stats.AllocMB,
		"sys_mb":      m.stats.SysMB,
		"num_gc":      m.stats.NumGC,
		"goroutines":  m.stats.Goroutines,
	}).Debug("Memory stats")

	// 警告: 内存使用超过 1.5GB
	if m.stats.AllocMB > 1536 {
		m.logger.WithFields(logrus.Fields{
			"alloc_mb": m.stats.AllocMB,
			"sys_mb":   m.stats.SysMB,
		}).Warn("High memory usage detected")
	}
}

// GetStats 获取当前统计信息
func (m *MemoryMonitor) GetStats() MemoryStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return *m.stats
}

// MetricsEndpoint 创建 Metrics 端点
func (m *MemoryMonitor) MetricsEndpoint() gin.HandlerFunc {
	return func(c *gin.Context) {
		stats := m.GetStats()
		c.JSON(200, gin.H{
			"memory": stats,
		})
	}
}

// ForceGC 手动触发 GC
func ForceGC() gin.HandlerFunc {
	return func(c *gin.Context) {
		runtime.GC()
		c.JSON(200, gin.H{
			"message": "GC triggered successfully",
		})
	}
}
