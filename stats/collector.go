package stats

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"file_uploader/config"
	"file_uploader/metrics"
	"file_uploader/storage"
)

// Stats 统计数据结构
type Stats struct {
	// 基本统计
	TotalFiles       int64     `json:"total_files"`
	TotalSize        int64     `json:"total_size"`
	TotalUploads     int64     `json:"total_uploads"`
	TotalDeletes     int64     `json:"total_deletes"`
	TotalAccesses    int64     `json:"total_accesses"`
	
	// 存储统计
	StorageStats     map[string]*StorageStats `json:"storage_stats"`
	
	// 时间统计
	LastUpdate       time.Time `json:"last_update"`
	Uptime           int64     `json:"uptime"`
	StartTime        time.Time `json:"start_time"`
	
	// 错误统计
	UploadErrors     int64     `json:"upload_errors"`
	DeleteErrors     int64     `json:"delete_errors"`
	AccessErrors     int64     `json:"access_errors"`
	
	// 批量操作统计
	BatchUploads     int64     `json:"batch_uploads"`
	BatchDeletes     int64     `json:"batch_deletes"`
	BatchErrors      int64     `json:"batch_errors"`
	
	// 图片处理统计
	ThumbnailsGenerated int64  `json:"thumbnails_generated"`
	ThumbnailErrors     int64  `json:"thumbnail_errors"`
	
	// 性能统计
	AvgUploadTime    float64   `json:"avg_upload_time"`
	AvgFileSize      float64   `json:"avg_file_size"`
	
	mu sync.RWMutex
}

// StorageStats 存储统计
type StorageStats struct {
	Type         string    `json:"type"`
	FileCount    int64     `json:"file_count"`
	TotalSize    int64     `json:"total_size"`
	LastAccess   time.Time `json:"last_access"`
	Enabled      bool      `json:"enabled"`
}

var (
	globalStats *Stats
	statsOnce   sync.Once
)

// GetStats 获取全局统计实例
func GetStats() *Stats {
	statsOnce.Do(func() {
		globalStats = &Stats{
			StorageStats: make(map[string]*StorageStats),
			StartTime:    time.Now(),
		}
	})
	return globalStats
}

// UpdateStorageStats 更新存储统计
func (s *Stats) UpdateStorageStats(storageManager *storage.StorageManager, cfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 更新默认存储统计
	if cfg.Storage.Type == "local" {
		s.updateLocalStorageStats("default", cfg.Storage.Local.UploadDir)
	}
	
	// 更新多存储统计
	if cfg.Storage.Storages != nil {
		for name, storageConfig := range cfg.Storage.Storages {
			// 检查存储是否启用
			enabled := true
			if len(cfg.Storage.EnabledStorages) > 0 {
				enabled = false
				for _, enabledName := range cfg.Storage.EnabledStorages {
					if enabledName == name {
						enabled = true
						break
					}
				}
			}
			
			if !enabled {
				continue
			}
			
			configMap, ok := storageConfig.(map[interface{}]interface{})
			if !ok {
				continue
			}
			
			storageTypeInterface, exists := configMap["type"]
			if !exists {
				continue
			}
			
			storageType, ok := storageTypeInterface.(string)
			if !ok {
				continue
			}
			
			if storageType == "local" {
				uploadDirInterface, exists := configMap["upload_dir"]
				if exists {
					if uploadDir, ok := uploadDirInterface.(string); ok {
						s.updateLocalStorageStats(name, uploadDir)
					}
				}
			} else if storageType == "s3" {
				// S3存储统计（暂时设置为0，实际使用时需要调用S3 API）
				if s.StorageStats[name] == nil {
					s.StorageStats[name] = &StorageStats{
						Type:    "s3",
						Enabled: true,
					}
				}
				s.StorageStats[name].LastAccess = time.Now()
			}
		}
	}
	
	s.LastUpdate = time.Now()
	s.Uptime = int64(time.Since(s.StartTime).Seconds())
	
	// 更新Prometheus指标
	m := metrics.GetMetrics()
	for name, stat := range s.StorageStats {
		m.UpdateStorageUsage(name, float64(stat.TotalSize))
	}
}

// updateLocalStorageStats 更新本地存储统计
func (s *Stats) updateLocalStorageStats(name, uploadDir string) {
	if s.StorageStats[name] == nil {
		s.StorageStats[name] = &StorageStats{
			Type:    "local",
			Enabled: true,
		}
	}
	
	var fileCount int64
	var totalSize int64
	
	if _, err := os.Stat(uploadDir); err == nil {
		err := filepath.WalkDir(uploadDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // 忽略错误，继续遍历
			}
			
			if !d.IsDir() {
				fileCount++
				if info, err := d.Info(); err == nil {
					totalSize += info.Size()
				}
			}
			return nil
		})
		
		if err != nil {
			log.Printf("遍历存储目录失败 %s: %v", uploadDir, err)
		}
	}
	
	s.StorageStats[name].FileCount = fileCount
	s.StorageStats[name].TotalSize = totalSize
	s.StorageStats[name].LastAccess = time.Now()
	
	// 更新总计
	s.TotalFiles = 0
	s.TotalSize = 0
	for _, stat := range s.StorageStats {
		s.TotalFiles += stat.FileCount
		s.TotalSize += stat.TotalSize
	}
}

// RecordUpload 记录上传
func (s *Stats) RecordUpload(size int64, duration time.Duration, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if success {
		s.TotalUploads++
		// 更新平均值
		if s.TotalUploads > 0 {
			s.AvgFileSize = (s.AvgFileSize*float64(s.TotalUploads-1) + float64(size)) / float64(s.TotalUploads)
			s.AvgUploadTime = (s.AvgUploadTime*float64(s.TotalUploads-1) + duration.Seconds()) / float64(s.TotalUploads)
		}
	} else {
		s.UploadErrors++
	}
}

// RecordDelete 记录删除
func (s *Stats) RecordDelete(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if success {
		s.TotalDeletes++
	} else {
		s.DeleteErrors++
	}
}

// RecordAccess 记录访问
func (s *Stats) RecordAccess(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if success {
		s.TotalAccesses++
	} else {
		s.AccessErrors++
	}
}

// RecordBatchUpload 记录批量上传
func (s *Stats) RecordBatchUpload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BatchUploads++
}

// RecordBatchDelete 记录批量删除
func (s *Stats) RecordBatchDelete() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BatchDeletes++
}

// RecordBatchError 记录批量错误
func (s *Stats) RecordBatchError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BatchErrors++
}

// RecordThumbnail 记录缩略图生成
func (s *Stats) RecordThumbnail(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if success {
		s.ThumbnailsGenerated++
	} else {
		s.ThumbnailErrors++
	}
}

// GetJSON 获取JSON格式的统计数据
func (s *Stats) GetJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return json.MarshalIndent(s, "", "  ")
}

// GetSummary 获取统计摘要
func (s *Stats) GetSummary() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return map[string]interface{}{
		"total_files":           s.TotalFiles,
		"total_size":            s.TotalSize,
		"total_size_mb":         float64(s.TotalSize) / 1024 / 1024,
		"total_uploads":         s.TotalUploads,
		"total_deletes":         s.TotalDeletes,
		"total_accesses":        s.TotalAccesses,
		"upload_errors":         s.UploadErrors,
		"delete_errors":         s.DeleteErrors,
		"access_errors":         s.AccessErrors,
		"batch_uploads":         s.BatchUploads,
		"batch_deletes":         s.BatchDeletes,
		"batch_errors":          s.BatchErrors,
		"thumbnails_generated":  s.ThumbnailsGenerated,
		"thumbnail_errors":      s.ThumbnailErrors,
		"avg_upload_time":       fmt.Sprintf("%.2fs", s.AvgUploadTime),
		"avg_file_size":         fmt.Sprintf("%.2fKB", s.AvgFileSize/1024),
		"uptime":                fmt.Sprintf("%.0fs", float64(s.Uptime)),
		"uptime_human":          time.Duration(s.Uptime * int64(time.Second)).String(),
		"storage_count":         len(s.StorageStats),
		"last_update":           s.LastUpdate.Format("2006-01-02 15:04:05"),
	}
}

// StartPeriodicUpdate 启动定期更新
func (s *Stats) StartPeriodicUpdate(storageManager *storage.StorageManager, cfg *config.Config, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			s.UpdateStorageStats(storageManager, cfg)
		}
	}()
}
