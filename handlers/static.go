package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"file_uploader/config"
	imageProcessor "file_uploader/image"
	"file_uploader/metrics"
	"file_uploader/middleware"
	"file_uploader/stats"
	"file_uploader/storage"
)

// StaticFileHandler 静态文件处理器
type StaticFileHandler struct {
	storageManager   *storage.StorageManager
	config           *config.Config
	imageOptimizer   *imageProcessor.ImageOptimizer
}

// NewStaticFileHandler 创建静态文件处理器
func NewStaticFileHandler(storageManager *storage.StorageManager, cfg *config.Config) *StaticFileHandler {
	return &StaticFileHandler{
		storageManager: storageManager,
		config:         cfg,
		imageOptimizer: imageProcessor.NewImageOptimizer(cfg),
	}
}

// ServeFile 处理静态文件请求
func (h *StaticFileHandler) ServeFile(storageName, uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		m := metrics.GetMetrics()
		statsCollector := stats.GetStats()

		// 检查该存储是否需要签名验证
		requireAuth := h.config.GetStorageAuthRequirement(storageName)

		if requireAuth {
			// 使用签名验证中间件
			authMiddleware := middleware.SignatureAuth()

			// 手动调用中间件验证
			authMiddleware(c)

			// 如果验证失败，中间件会自动返回错误
			if c.IsAborted() {
				m.RecordFileAccess(false)
				statsCollector.RecordAccess(false)
				return
			}
		}

		// 获取文件路径
		filePath := c.Param("filepath")
		if filePath == "" {
			m.RecordFileAccess(false)
			statsCollector.RecordAccess(false)
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件路径不能为空"})
			return
		}

		// 移除开头的斜杠
		if strings.HasPrefix(filePath, "/") {
			filePath = filePath[1:]
		}

		// 构建完整的文件路径
		fullPath := filepath.Join(uploadDir, filePath)

		// 安全检查：防止路径遍历攻击
		cleanUploadDir := filepath.Clean(uploadDir)
		cleanFullPath := filepath.Clean(fullPath)
		if !strings.HasPrefix(cleanFullPath, cleanUploadDir) {
			m.RecordFileAccess(false)
			statsCollector.RecordAccess(false)
			c.JSON(http.StatusForbidden, gin.H{"error": "非法的文件路径"})
			return
		}

		// 获取存储实例来检查文件是否存在
		storageInstance, err := h.storageManager.GetStorage(storageName)
		if err == nil && storageInstance != nil {
			exists, err := storageInstance.Exists(filePath)
			if err != nil {
				m.RecordFileAccess(false)
				statsCollector.RecordAccess(false)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "检查文件失败"})
				return
			}

			if !exists {
				m.RecordFileAccess(false)
				statsCollector.RecordAccess(false)
				c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
				return
			}
		}

		// 记录成功访问
		m.RecordFileAccess(true)
		statsCollector.RecordAccess(true)

		// 检查是否需要图片优化
		optimizeParams := h.imageOptimizer.ParseOptimizeParams(c)
		if h.imageOptimizer.ShouldOptimize(optimizeParams, fullPath) {
			h.serveOptimizedImage(c, fullPath, optimizeParams)
			return
		}

		// 提供原始文件服务
		c.File(fullPath)
	}
}

// ServeFilePublic 处理公开静态文件请求（无需签名验证但包含访问统计）
func (h *StaticFileHandler) ServeFilePublic(storageName, uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		m := metrics.GetMetrics()
		statsCollector := stats.GetStats()

		// 获取文件路径
		filePath := c.Param("filepath")
		if filePath == "" {
			m.RecordFileAccess(false)
			statsCollector.RecordAccess(false)
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件路径不能为空"})
			return
		}

		// 移除开头的斜杠
		if strings.HasPrefix(filePath, "/") {
			filePath = filePath[1:]
		}

		// 构建完整的文件路径
		fullPath := filepath.Join(uploadDir, filePath)

		// 安全检查：防止路径遍历攻击
		cleanUploadDir := filepath.Clean(uploadDir)
		cleanFullPath := filepath.Clean(fullPath)
		if !strings.HasPrefix(cleanFullPath, cleanUploadDir) {
			m.RecordFileAccess(false)
			statsCollector.RecordAccess(false)
			c.JSON(http.StatusForbidden, gin.H{"error": "非法的文件路径"})
			return
		}

		// 检查文件是否存在
		if storageName != "" {
			// 使用存储实例检查
			storageInstance, err := h.storageManager.GetStorage(storageName)
			if err == nil && storageInstance != nil {
				exists, err := storageInstance.Exists(filePath)
				if err != nil {
					m.RecordFileAccess(false)
					statsCollector.RecordAccess(false)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "检查文件失败"})
					return
				}

				if !exists {
					m.RecordFileAccess(false)
					statsCollector.RecordAccess(false)
					c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
					return
				}
			}
		} else {
			// 直接检查文件系统
			if _, err := filepath.Abs(fullPath); err != nil {
				m.RecordFileAccess(false)
				statsCollector.RecordAccess(false)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "文件路径错误"})
				return
			}
		}

		// 记录成功访问
		m.RecordFileAccess(true)
		statsCollector.RecordAccess(true)

		// 检查是否需要图片优化
		optimizeParams := h.imageOptimizer.ParseOptimizeParams(c)
		if h.imageOptimizer.ShouldOptimize(optimizeParams, fullPath) {
			h.serveOptimizedImage(c, fullPath, optimizeParams)
			return
		}

		// 提供原始文件服务
		c.File(fullPath)
	}
}

// serveOptimizedImage 提供优化后的图片
func (h *StaticFileHandler) serveOptimizedImage(c *gin.Context, filePath string, params *imageProcessor.OptimizeParams) {
	// 打开原始文件
	file, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开文件"})
		return
	}
	defer file.Close()

	// 优化图片
	data, contentType, err := h.imageOptimizer.OptimizeImage(file, params)
	if err != nil {
		// 如果优化失败，返回原始文件
		c.File(filePath)
		return
	}

	// 设置响应头
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=31536000") // 缓存1年

	// 返回优化后的图片数据
	c.Data(http.StatusOK, contentType, data)
}
