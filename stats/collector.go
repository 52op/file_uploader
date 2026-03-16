package stats

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"file_uploader/config"
	"file_uploader/metrics"
	"file_uploader/storage"
)

// Stats 统计数据结构
type Stats struct {
	// 基本统计
	TotalFiles    int64 `json:"total_files"`
	TotalSize     int64 `json:"total_size"`
	TotalUploads  int64 `json:"total_uploads"`
	TotalDeletes  int64 `json:"total_deletes"`
	TotalAccesses int64 `json:"total_accesses"`

	// 存储统计
	StorageStats map[string]*StorageStats `json:"storage_stats"`

	// 时间统计
	LastUpdate time.Time `json:"last_update"`
	Uptime     int64     `json:"uptime"`
	StartTime  time.Time `json:"start_time"`

	// 错误统计
	UploadErrors int64 `json:"upload_errors"`
	DeleteErrors int64 `json:"delete_errors"`
	AccessErrors int64 `json:"access_errors"`

	// 批量操作统计
	BatchUploads int64 `json:"batch_uploads"`
	BatchDeletes int64 `json:"batch_deletes"`
	BatchErrors  int64 `json:"batch_errors"`

	// 图片处理统计
	ThumbnailsGenerated int64 `json:"thumbnails_generated"`
	ThumbnailErrors     int64 `json:"thumbnail_errors"`

	// 性能统计
	AvgUploadTime float64 `json:"avg_upload_time"`
	AvgFileSize   float64 `json:"avg_file_size"`

	mu          sync.RWMutex
	persistence *Persistence
	dirty       bool
}

// Snapshot 统计快照
type Snapshot struct {
	Timestamp           time.Time                `json:"timestamp"`
	TotalFiles          int64                    `json:"total_files"`
	TotalSize           int64                    `json:"total_size"`
	TotalUploads        int64                    `json:"total_uploads"`
	TotalDeletes        int64                    `json:"total_deletes"`
	TotalAccesses       int64                    `json:"total_accesses"`
	UploadErrors        int64                    `json:"upload_errors"`
	DeleteErrors        int64                    `json:"delete_errors"`
	AccessErrors        int64                    `json:"access_errors"`
	BatchUploads        int64                    `json:"batch_uploads"`
	BatchDeletes        int64                    `json:"batch_deletes"`
	BatchErrors         int64                    `json:"batch_errors"`
	ThumbnailsGenerated int64                    `json:"thumbnails_generated"`
	ThumbnailErrors     int64                    `json:"thumbnail_errors"`
	AvgUploadTime       float64                  `json:"avg_upload_time"`
	AvgFileSize         float64                  `json:"avg_file_size"`
	LastUpdate          time.Time                `json:"last_update"`
	StorageStats        map[string]*StorageStats `json:"storage_stats,omitempty"`
}

// Persistence 统计持久化管理
type Persistence struct {
	dataDir          string
	currentFile      string
	historyFile      string
	flushInterval    time.Duration
	snapshotInterval time.Duration
	retentionDays    int
}

// StorageStats 存储统计
type StorageStats struct {
	Type       string    `json:"type"`
	FileCount  int64     `json:"file_count"`
	TotalSize  int64     `json:"total_size"`
	LastAccess time.Time `json:"last_access"`
	Enabled    bool      `json:"enabled"`
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

// InitPersistence 初始化统计持久化
func (s *Stats) InitPersistence(cfg *config.Config) error {
	if !cfg.Stats.Enabled {
		return nil
	}

	flushInterval, err := time.ParseDuration(cfg.Stats.FlushInterval)
	if err != nil {
		return fmt.Errorf("解析统计刷新间隔失败: %v", err)
	}

	snapshotInterval, err := time.ParseDuration(cfg.Stats.SnapshotInterval)
	if err != nil {
		return fmt.Errorf("解析统计快照间隔失败: %v", err)
	}

	persistence := &Persistence{
		dataDir:          cfg.Stats.DataDir,
		currentFile:      filepath.Join(cfg.Stats.DataDir, "current.json"),
		historyFile:      filepath.Join(cfg.Stats.DataDir, "history.jsonl"),
		flushInterval:    flushInterval,
		snapshotInterval: snapshotInterval,
		retentionDays:    cfg.Stats.RetentionDays,
	}

	if err := os.MkdirAll(persistence.dataDir, 0755); err != nil {
		return fmt.Errorf("创建统计目录失败: %v", err)
	}

	if err := s.loadCurrentSnapshot(persistence.currentFile); err != nil {
		return err
	}

	s.mu.Lock()
	s.persistence = persistence
	s.mu.Unlock()

	return nil
}

// StartPersistence 启动统计持久化任务
func (s *Stats) StartPersistence(storageManager *storage.StorageManager, cfg *config.Config) {
	s.mu.RLock()
	persistence := s.persistence
	s.mu.RUnlock()
	if persistence == nil {
		return
	}

	go func() {
		flushTicker := time.NewTicker(persistence.flushInterval)
		snapshotTicker := time.NewTicker(persistence.snapshotInterval)
		defer flushTicker.Stop()
		defer snapshotTicker.Stop()

		for {
			select {
			case <-flushTicker.C:
				if err := s.flushCurrentSnapshot(); err != nil {
					log.Printf("持久化统计当前快照失败: %v", err)
				}
			case <-snapshotTicker.C:
				s.UpdateStorageStats(storageManager, cfg)
				if err := s.appendHistorySnapshot(); err != nil {
					log.Printf("写入统计历史快照失败: %v", err)
				}
			}
		}
	}()
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
	s.dirty = true

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
	s.LastUpdate = time.Now()
	s.dirty = true
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
	s.LastUpdate = time.Now()
	s.dirty = true
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
	s.LastUpdate = time.Now()
	s.dirty = true
}

// RecordBatchUpload 记录批量上传
func (s *Stats) RecordBatchUpload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BatchUploads++
	s.LastUpdate = time.Now()
	s.dirty = true
}

// RecordBatchDelete 记录批量删除
func (s *Stats) RecordBatchDelete() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BatchDeletes++
	s.LastUpdate = time.Now()
	s.dirty = true
}

// RecordBatchError 记录批量错误
func (s *Stats) RecordBatchError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BatchErrors++
	s.LastUpdate = time.Now()
	s.dirty = true
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
	s.LastUpdate = time.Now()
	s.dirty = true
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
		"total_files":          s.TotalFiles,
		"total_size":           s.TotalSize,
		"total_size_mb":        float64(s.TotalSize) / 1024 / 1024,
		"total_uploads":        s.TotalUploads,
		"total_deletes":        s.TotalDeletes,
		"total_accesses":       s.TotalAccesses,
		"upload_errors":        s.UploadErrors,
		"delete_errors":        s.DeleteErrors,
		"access_errors":        s.AccessErrors,
		"batch_uploads":        s.BatchUploads,
		"batch_deletes":        s.BatchDeletes,
		"batch_errors":         s.BatchErrors,
		"thumbnails_generated": s.ThumbnailsGenerated,
		"thumbnail_errors":     s.ThumbnailErrors,
		"avg_upload_time":      fmt.Sprintf("%.2fs", s.AvgUploadTime),
		"avg_file_size":        fmt.Sprintf("%.2fKB", s.AvgFileSize/1024),
		"uptime":               fmt.Sprintf("%.0fs", float64(s.Uptime)),
		"uptime_human":         time.Duration(s.Uptime * int64(time.Second)).String(),
		"storage_count":        len(s.StorageStats),
		"last_update":          s.LastUpdate.Format("2006-01-02 15:04:05"),
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

// GetHistory 获取历史统计快照
func (s *Stats) GetHistory(from, to time.Time, limit int) ([]Snapshot, error) {
	s.mu.RLock()
	persistence := s.persistence
	current := s.snapshotLocked(time.Now())
	s.mu.RUnlock()
	if persistence == nil {
		return []Snapshot{current}, nil
	}

	file, err := os.Open(persistence.historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []Snapshot{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var items []Snapshot
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var snapshot Snapshot
		if err := json.Unmarshal(line, &snapshot); err != nil {
			continue
		}
		if !from.IsZero() && snapshot.Timestamp.Before(from) {
			continue
		}
		if !to.IsZero() && snapshot.Timestamp.After(to) {
			continue
		}
		items = append(items, snapshot)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}

	if (from.IsZero() || !current.Timestamp.Before(from)) && (to.IsZero() || !current.Timestamp.After(to)) {
		if len(items) == 0 || !items[len(items)-1].Timestamp.Equal(current.Timestamp) {
			items = append(items, current)
		}
	}

	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}

	return items, nil
}

func (s *Stats) flushCurrentSnapshot() error {
	s.mu.RLock()
	persistence := s.persistence
	dirty := s.dirty
	snapshot := s.snapshotLocked(time.Now())
	s.mu.RUnlock()

	if persistence == nil || !dirty {
		return nil
	}

	if err := writeJSONFile(persistence.currentFile, snapshot); err != nil {
		return err
	}

	s.mu.Lock()
	if !s.LastUpdate.After(snapshot.LastUpdate) {
		s.dirty = false
	}
	s.mu.Unlock()
	return nil
}

func (s *Stats) appendHistorySnapshot() error {
	s.mu.RLock()
	persistence := s.persistence
	snapshot := s.snapshotLocked(time.Now())
	s.mu.RUnlock()

	if persistence == nil {
		return nil
	}

	if err := writeJSONFile(persistence.currentFile, snapshot); err != nil {
		return err
	}
	if err := appendJSONLine(persistence.historyFile, snapshot); err != nil {
		return err
	}
	if err := pruneHistory(persistence.historyFile, persistence.retentionDays); err != nil {
		return err
	}

	s.mu.Lock()
	if !s.LastUpdate.After(snapshot.LastUpdate) {
		s.dirty = false
	}
	s.mu.Unlock()
	return nil
}

func (s *Stats) loadCurrentSnapshot(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取统计快照失败: %v", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("解析统计快照失败: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.applySnapshotLocked(snapshot)
	return nil
}

func (s *Stats) applySnapshotLocked(snapshot Snapshot) {
	s.TotalFiles = snapshot.TotalFiles
	s.TotalSize = snapshot.TotalSize
	s.TotalUploads = snapshot.TotalUploads
	s.TotalDeletes = snapshot.TotalDeletes
	s.TotalAccesses = snapshot.TotalAccesses
	s.UploadErrors = snapshot.UploadErrors
	s.DeleteErrors = snapshot.DeleteErrors
	s.AccessErrors = snapshot.AccessErrors
	s.BatchUploads = snapshot.BatchUploads
	s.BatchDeletes = snapshot.BatchDeletes
	s.BatchErrors = snapshot.BatchErrors
	s.ThumbnailsGenerated = snapshot.ThumbnailsGenerated
	s.ThumbnailErrors = snapshot.ThumbnailErrors
	s.AvgUploadTime = snapshot.AvgUploadTime
	s.AvgFileSize = snapshot.AvgFileSize
	s.LastUpdate = snapshot.LastUpdate
	s.StorageStats = cloneStorageStats(snapshot.StorageStats)
	s.dirty = false
}

func (s *Stats) snapshotLocked(now time.Time) Snapshot {
	storageStats := cloneStorageStats(s.StorageStats)
	return Snapshot{
		Timestamp:           now,
		TotalFiles:          s.TotalFiles,
		TotalSize:           s.TotalSize,
		TotalUploads:        s.TotalUploads,
		TotalDeletes:        s.TotalDeletes,
		TotalAccesses:       s.TotalAccesses,
		UploadErrors:        s.UploadErrors,
		DeleteErrors:        s.DeleteErrors,
		AccessErrors:        s.AccessErrors,
		BatchUploads:        s.BatchUploads,
		BatchDeletes:        s.BatchDeletes,
		BatchErrors:         s.BatchErrors,
		ThumbnailsGenerated: s.ThumbnailsGenerated,
		ThumbnailErrors:     s.ThumbnailErrors,
		AvgUploadTime:       s.AvgUploadTime,
		AvgFileSize:         s.AvgFileSize,
		LastUpdate:          s.LastUpdate,
		StorageStats:        storageStats,
	}
}

func cloneStorageStats(src map[string]*StorageStats) map[string]*StorageStats {
	if len(src) == 0 {
		return map[string]*StorageStats{}
	}
	dst := make(map[string]*StorageStats, len(src))
	for name, stat := range src {
		if stat == nil {
			continue
		}
		copyStat := *stat
		dst[name] = &copyStat
	}
	return dst
}

func writeJSONFile(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tempPath, path)
}

func appendJSONLine(path string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func pruneHistory(path string, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	lines := strings.Split(string(data), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var snapshot Snapshot
		if err := json.Unmarshal([]byte(line), &snapshot); err != nil {
			continue
		}
		if snapshot.Timestamp.Before(cutoff) {
			continue
		}
		filtered = append(filtered, line)
	}

	return os.WriteFile(path, []byte(strings.Join(filtered, "\n")+"\n"), 0644)
}
