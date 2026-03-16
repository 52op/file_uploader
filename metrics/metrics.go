package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 包含所有Prometheus指标
type Metrics struct {
	// 文件上传相关指标
	UploadTotal       prometheus.Counter
	UploadDuration    prometheus.Histogram
	UploadSize        prometheus.Histogram
	UploadErrors      prometheus.Counter
	
	// 文件删除相关指标
	DeleteTotal       prometheus.Counter
	DeleteErrors      prometheus.Counter
	
	// 文件访问相关指标
	FileAccessTotal   prometheus.Counter
	FileAccessErrors  prometheus.Counter
	
	// 存储相关指标
	StorageUsage      *prometheus.GaugeVec
	StorageErrors     *prometheus.CounterVec

	// API相关指标
	HTTPRequestsTotal *prometheus.CounterVec
	HTTPDuration      *prometheus.HistogramVec
	
	// 系统相关指标
	ActiveConnections prometheus.Gauge
	StartTime         prometheus.Gauge
	
	// 批量操作指标
	BatchUploadTotal  prometheus.Counter
	BatchDeleteTotal  prometheus.Counter
	BatchErrors       prometheus.Counter
	
	// 图片处理指标
	ThumbnailGenerated prometheus.Counter
	ThumbnailErrors    prometheus.Counter
	ThumbnailDuration  prometheus.Histogram
}

var (
	instance *Metrics
	once     sync.Once
)

// GetMetrics 获取指标实例（单例模式）
func GetMetrics() *Metrics {
	once.Do(func() {
		instance = &Metrics{
			// 文件上传指标
			UploadTotal: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_uploads_total",
				Help: "Total number of file uploads",
			}),
			UploadDuration: promauto.NewHistogram(prometheus.HistogramOpts{
				Name:    "file_uploader_upload_duration_seconds",
				Help:    "Duration of file uploads in seconds",
				Buckets: prometheus.DefBuckets,
			}),
			UploadSize: promauto.NewHistogram(prometheus.HistogramOpts{
				Name:    "file_uploader_upload_size_bytes",
				Help:    "Size of uploaded files in bytes",
				Buckets: []float64{1024, 10240, 102400, 1048576, 10485760, 104857600, 1073741824}, // 1KB to 1GB
			}),
			UploadErrors: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_upload_errors_total",
				Help: "Total number of upload errors",
			}),
			
			// 文件删除指标
			DeleteTotal: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_deletes_total",
				Help: "Total number of file deletions",
			}),
			DeleteErrors: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_delete_errors_total",
				Help: "Total number of delete errors",
			}),
			
			// 文件访问指标
			FileAccessTotal: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_file_access_total",
				Help: "Total number of file accesses",
			}),
			FileAccessErrors: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_file_access_errors_total",
				Help: "Total number of file access errors",
			}),
			
			// 存储指标
			StorageUsage: promauto.NewGaugeVec(prometheus.GaugeOpts{
				Name: "file_uploader_storage_usage_bytes",
				Help: "Storage usage in bytes by storage type",
			}, []string{"storage_type"}),
			StorageErrors: promauto.NewCounterVec(prometheus.CounterOpts{
				Name: "file_uploader_storage_errors_total",
				Help: "Total number of storage errors by storage type",
			}, []string{"storage_type", "operation"}),
			
			// API指标
			HTTPRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
				Name: "file_uploader_http_requests_total",
				Help: "Total number of HTTP requests",
			}, []string{"method", "endpoint", "status"}),
			HTTPDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
				Name:    "file_uploader_http_duration_seconds",
				Help:    "Duration of HTTP requests in seconds",
				Buckets: prometheus.DefBuckets,
			}, []string{"method", "endpoint"}),
			
			// 系统指标
			ActiveConnections: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "file_uploader_active_connections",
				Help: "Number of active connections",
			}),
			StartTime: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "file_uploader_start_time_seconds",
				Help: "Start time of the application in unix timestamp",
			}),
			
			// 批量操作指标
			BatchUploadTotal: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_batch_uploads_total",
				Help: "Total number of batch uploads",
			}),
			BatchDeleteTotal: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_batch_deletes_total",
				Help: "Total number of batch deletions",
			}),
			BatchErrors: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_batch_errors_total",
				Help: "Total number of batch operation errors",
			}),
			
			// 图片处理指标
			ThumbnailGenerated: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_thumbnails_generated_total",
				Help: "Total number of thumbnails generated",
			}),
			ThumbnailErrors: promauto.NewCounter(prometheus.CounterOpts{
				Name: "file_uploader_thumbnail_errors_total",
				Help: "Total number of thumbnail generation errors",
			}),
			ThumbnailDuration: promauto.NewHistogram(prometheus.HistogramOpts{
				Name:    "file_uploader_thumbnail_duration_seconds",
				Help:    "Duration of thumbnail generation in seconds",
				Buckets: prometheus.DefBuckets,
			}),
		}
		
		// 设置启动时间
		instance.StartTime.Set(float64(time.Now().Unix()))
	})
	return instance
}

// RecordUpload 记录文件上传
func (m *Metrics) RecordUpload(duration time.Duration, size int64, success bool) {
	if success {
		m.UploadTotal.Inc()
		m.UploadDuration.Observe(duration.Seconds())
		m.UploadSize.Observe(float64(size))
	} else {
		m.UploadErrors.Inc()
	}
}

// RecordDelete 记录文件删除
func (m *Metrics) RecordDelete(success bool) {
	if success {
		m.DeleteTotal.Inc()
	} else {
		m.DeleteErrors.Inc()
	}
}

// RecordFileAccess 记录文件访问
func (m *Metrics) RecordFileAccess(success bool) {
	if success {
		m.FileAccessTotal.Inc()
	} else {
		m.FileAccessErrors.Inc()
	}
}

// RecordStorageError 记录存储错误
func (m *Metrics) RecordStorageError(storageType, operation string) {
	m.StorageErrors.WithLabelValues(storageType, operation).Inc()
}

// UpdateStorageUsage 更新存储使用量
func (m *Metrics) UpdateStorageUsage(storageType string, usage float64) {
	m.StorageUsage.WithLabelValues(storageType).Set(usage)
}

// RecordHTTPRequest 记录HTTP请求
func (m *Metrics) RecordHTTPRequest(method, endpoint, status string, duration time.Duration) {
	m.HTTPRequestsTotal.WithLabelValues(method, endpoint, status).Inc()
	m.HTTPDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}

// UpdateActiveConnections 更新活跃连接数
func (m *Metrics) UpdateActiveConnections(count float64) {
	m.ActiveConnections.Set(count)
}

// RecordBatchUpload 记录批量上传
func (m *Metrics) RecordBatchUpload() {
	m.BatchUploadTotal.Inc()
}

// RecordBatchDelete 记录批量删除
func (m *Metrics) RecordBatchDelete() {
	m.BatchDeleteTotal.Inc()
}

// RecordBatchError 记录批量操作错误
func (m *Metrics) RecordBatchError() {
	m.BatchErrors.Inc()
}

// RecordThumbnail 记录缩略图生成
func (m *Metrics) RecordThumbnail(duration time.Duration, success bool) {
	if success {
		m.ThumbnailGenerated.Inc()
		m.ThumbnailDuration.Observe(duration.Seconds())
	} else {
		m.ThumbnailErrors.Inc()
	}
}
