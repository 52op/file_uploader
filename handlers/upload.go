package handlers

import (
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"file_uploader/config"
	imageProcessor "file_uploader/image"
	"file_uploader/metrics"
	"file_uploader/stats"
	"file_uploader/storage"
)

// UploadHandler 文件上传处理器结构
type UploadHandler struct {
	storageManager   *storage.StorageManager
	config          *config.Config
	imageProcessor  *imageProcessor.ImageProcessor
}

// NewUploadHandler 创建上传处理器实例
func NewUploadHandler(storageManager *storage.StorageManager, cfg *config.Config) *UploadHandler {
	return &UploadHandler{
		storageManager:  storageManager,
		config:         cfg,
		imageProcessor: imageProcessor.NewImageProcessor(cfg),
	}
}

// UploadFile 处理文件上传请求
func (h *UploadHandler) UploadFile(c *gin.Context) {
	start := time.Now()
	m := metrics.GetMetrics()
	statsCollector := stats.GetStats()

	// 获取上传的文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		m.RecordUpload(time.Since(start), 0, false)
		statsCollector.RecordUpload(0, time.Since(start), false)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "获取上传文件失败",
			"details": err.Error(),
		})
		return
	}
	defer file.Close()

	// 验证文件
	if err := validateFile(header, h.config); err != nil {
		m.RecordUpload(time.Since(start), header.Size, false)
		statsCollector.RecordUpload(header.Size, time.Since(start), false)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "文件验证失败",
			"details": err.Error(),
		})
		return
	}

	// 获取自定义路径参数
	customPath := c.PostForm("path")
	storageType := c.PostForm("storage")

	// 生成文件名
	var filename string
	if customPath != "" {
		// 使用自定义路径
		filename = strings.TrimPrefix(customPath, "/")
		// 如果路径不包含文件名，则添加原始文件名
		if strings.HasSuffix(filename, "/") || filename == "" {
			filename = filepath.Join(filename, generateFilename(header.Filename, h.config))
		}
	} else {
		// 使用默认文件名生成策略
		filename = generateFilename(header.Filename, h.config)
	}

	// 解析存储类型和文件路径
	var targetStorageType, targetFilePath string
	if storageType != "" {
		// 使用指定的存储类型
		targetStorageType = storageType
		targetFilePath = filename
	} else {
		// 从路径中解析存储类型
		targetStorageType, targetFilePath = h.storageManager.ParseUploadPath(filename)
	}

	// 获取存储实例
	storageInstance, err := h.storageManager.GetStorage(targetStorageType)
	if err != nil {
		m.RecordUpload(time.Since(start), header.Size, false)
		m.RecordStorageError(targetStorageType, "get_storage")
		statsCollector.RecordUpload(header.Size, time.Since(start), false)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "存储类型不支持",
			"details": err.Error(),
		})
		return
	}

	// 处理图片（如果是图片文件）
	var thumbnailURL string
	var imageResult *imageProcessor.ImageUploadResult
	if imageProcessor.IsImageFile(targetFilePath) {
		// 重置文件指针到开始位置
		if seeker, ok := file.(io.Seeker); ok {
			seeker.Seek(0, io.SeekStart)
		}

		// 处理图片上传
		imageResult, err = h.imageProcessor.ProcessImageUpload(file, targetFilePath, header.Size, h.config)
		if err != nil {
			log.Printf("图片处理失败: %v", err)
		} else if imageResult.HasThumbnail {
			// 上传缩略图
			thumbnailPath := imageResult.ThumbnailFilename
			if storageType != "" {
				// 如果指定了存储类型，缩略图使用相同的存储
				thumbnailPath = imageResult.ThumbnailFilename
			} else {
				// 从路径中解析存储类型
				_, thumbnailPath = h.storageManager.ParseUploadPath(imageResult.ThumbnailFilename)
			}

			thumbnailURL, err = storageInstance.UploadReader(thumbnailPath, imageResult.ThumbnailReader)
			if err != nil {
				log.Printf("缩略图上传失败: %v", err)
				// 缩略图上传失败，标记使用原图作为缩略图
				imageResult.UseOriginalAsThumbnail = true
			} else {
				log.Printf("缩略图上传成功: %s", thumbnailURL)
			}
		}

		// 重置文件指针到开始位置以便上传原图
		if seeker, ok := file.(io.Seeker); ok {
			seeker.Seek(0, io.SeekStart)
		}
	}

	// 上传文件到存储
	fileURL, err := storageInstance.Upload(targetFilePath, file)
	if err != nil {
		m.RecordUpload(time.Since(start), header.Size, false)
		m.RecordStorageError(targetStorageType, "upload")
		statsCollector.RecordUpload(header.Size, time.Since(start), false)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "文件上传失败",
			"details": err.Error(),
		})
		return
	}

	// 记录成功的上传
	duration := time.Since(start)
	m.RecordUpload(duration, header.Size, true)
	statsCollector.RecordUpload(header.Size, duration, true)

	// 构建响应
	response := storage.UploadResult{
		Success: true,
		FileInfo: storage.FileInfo{
			Filename:    targetFilePath,
			Size:        header.Size,
			ContentType: header.Header.Get("Content-Type"),
			URL:         fileURL,
			UploadTime:  time.Now().Unix(),
		},
		Message:     "文件上传成功",
		StorageType: targetStorageType,
	}

	// 如果有缩略图，添加到响应中
	if thumbnailURL != "" {
		response.ThumbnailURL = thumbnailURL
	} else if imageResult != nil && imageResult.IsImage && (imageResult.UseOriginalAsThumbnail || !h.config.Thumbnail.Enabled) {
		// 如果是图片但不满足缩略图生成条件，或缩略图功能被禁用，使用原图作为缩略图
		response.ThumbnailURL = fileURL
	}

	c.JSON(http.StatusOK, response)
}

// DeleteFile 处理文件删除请求
func (h *UploadHandler) DeleteFile(c *gin.Context) {
	log.Printf("[DELETE] 收到删除请求: %s", c.Request.URL.Path)
	m := metrics.GetMetrics()
	statsCollector := stats.GetStats()

	filename := c.Param("filepath")
	log.Printf("[DELETE] 原始filepath参数: %s", filename)
	// 移除开头的斜杠（gin的*filepath参数会包含开头的斜杠）
	if strings.HasPrefix(filename, "/") {
		filename = filename[1:]
	}
	log.Printf("[DELETE] 处理后的filename: %s", filename)

	if filename == "" {
		m.RecordDelete(false)
		statsCollector.RecordDelete(false)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "文件名不能为空",
		})
		return
	}

	// 获取存储类型参数
	storageType := c.Query("storage")

	// 解析存储类型和文件路径
	var targetStorageType, targetFilePath string
	if storageType != "" {
		// 使用查询参数指定的存储类型
		targetStorageType = storageType
		// 需要从filename中移除存储类型前缀
		_, targetFilePath = h.storageManager.ParseUploadPath(filename)
		log.Printf("[DELETE] 使用查询参数存储类型: %s, 解析后文件路径: %s", targetStorageType, targetFilePath)
	} else {
		// 从路径中解析存储类型和文件路径
		targetStorageType, targetFilePath = h.storageManager.ParseUploadPath(filename)
		log.Printf("[DELETE] 解析路径结果: 存储类型=%s, 文件路径=%s", targetStorageType, targetFilePath)
	}

	// 获取存储实例
	storageInstance, err := h.storageManager.GetStorage(targetStorageType)
	if err != nil {
		m.RecordDelete(false)
		m.RecordStorageError(targetStorageType, "get_storage")
		statsCollector.RecordDelete(false)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "存储类型不支持",
			"details": err.Error(),
		})
		return
	}

	// 检查文件是否存在
	log.Printf("[DELETE] 检查文件是否存在: %s", targetFilePath)
	exists, err := storageInstance.Exists(targetFilePath)
	if err != nil {
		log.Printf("[DELETE] 检查文件存在性失败: %v", err)
		m.RecordDelete(false)
		m.RecordStorageError(targetStorageType, "exists")
		statsCollector.RecordDelete(false)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "检查文件存在性失败",
			"details": err.Error(),
		})
		return
	}

	log.Printf("[DELETE] 文件存在性检查结果: %v", exists)
	if !exists {
		log.Printf("[DELETE] 文件不存在: %s", targetFilePath)
		m.RecordDelete(false)
		statsCollector.RecordDelete(false)
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "文件不存在",
		})
		return
	}

	// 删除原文件
	if err := storageInstance.Delete(targetFilePath); err != nil {
		m.RecordDelete(false)
		m.RecordStorageError(targetStorageType, "delete")
		statsCollector.RecordDelete(false)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "文件删除失败",
			"details": err.Error(),
		})
		return
	}

	// 尝试删除对应的缩略图
	thumbnailPath := generateThumbnailPath(targetFilePath)
	log.Printf("[DELETE] 检查缩略图: %s", thumbnailPath)

	thumbnailExists, err := storageInstance.Exists(thumbnailPath)
	if err == nil && thumbnailExists {
		if err := storageInstance.Delete(thumbnailPath); err != nil {
			log.Printf("[DELETE] 缩略图删除失败: %v", err)
			// 缩略图删除失败不影响主文件删除的成功状态
		} else {
			log.Printf("[DELETE] 缩略图删除成功: %s", thumbnailPath)
		}
	} else if err != nil {
		log.Printf("[DELETE] 检查缩略图存在性失败: %v", err)
	} else {
		log.Printf("[DELETE] 缩略图不存在: %s", thumbnailPath)
	}

	// 记录成功删除
	m.RecordDelete(true)
	statsCollector.RecordDelete(true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "文件删除成功",
	})
}

// generateThumbnailPath 生成缩略图路径
// 例如: images/test.jpg -> images/test_Thumbnail.jpg
func generateThumbnailPath(originalPath string) string {
	ext := filepath.Ext(originalPath)
	nameWithoutExt := strings.TrimSuffix(originalPath, ext)
	return nameWithoutExt + "_Thumbnail" + ext
}

// GetFileInfo 获取文件信息
func (h *UploadHandler) GetFileInfo(c *gin.Context) {
	filename := c.Param("filepath")
	// 移除开头的斜杠（gin的*filepath参数会包含开头的斜杠）
	if strings.HasPrefix(filename, "/") {
		filename = filename[1:]
	}

	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "文件名不能为空",
		})
		return
	}

	// 获取存储类型参数
	storageType := c.Query("storage")

	// 解析存储类型和文件路径
	var targetStorageType, targetFilePath string
	if storageType != "" {
		targetStorageType = storageType
		targetFilePath = filename
	} else {
		targetStorageType, targetFilePath = h.storageManager.ParseUploadPath(filename)
	}

	// 获取存储实例
	storageInstance, err := h.storageManager.GetStorage(targetStorageType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "存储类型不支持",
			"details": err.Error(),
		})
		return
	}

	// 检查文件是否存在
	exists, err := storageInstance.Exists(targetFilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "检查文件存在性失败",
			"details": err.Error(),
		})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "文件不存在",
		})
		return
	}

	// 获取文件大小
	size, err := storageInstance.GetFileSize(targetFilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取文件大小失败",
			"details": err.Error(),
		})
		return
	}

	// 构建文件信息
	fileInfo := storage.FileInfo{
		Filename: targetFilePath,
		Size:     size,
		URL:      storageInstance.GetURL(targetFilePath),
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"file_info":    fileInfo,
		"storage_type": targetStorageType,
	})
}

// validateFile 验证上传的文件
func validateFile(header *multipart.FileHeader, cfg *config.Config) error {
	// 检查文件大小
	maxSize := cfg.Upload.MaxFileSize
	if maxSize <= 0 {
		maxSize = 100 * 1024 * 1024 // 默认100MB
	}
	if header.Size > maxSize {
		return fmt.Errorf("文件大小超过限制，最大允许 %d MB", maxSize/(1024*1024))
	}

	// 检查文件名
	if header.Filename == "" {
		return fmt.Errorf("文件名不能为空")
	}

	// 检查文件扩展名
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		return fmt.Errorf("文件必须有扩展名")
	}

	// 从配置获取允许的文件类型
	if len(cfg.Upload.AllowedExtensions) > 0 {
		// 如果配置中有设置，使用配置的扩展名列表（支持通配符）
		if !isFileTypeAllowed(header.Filename, cfg.Upload.AllowedExtensions) {
			return fmt.Errorf("不支持的文件类型: %s", ext)
		}
	} else {
		// 使用默认的扩展名列表
		allowedExts := getDefaultAllowedExtensions()
		if !allowedExts[ext] {
			return fmt.Errorf("不支持的文件类型: %s", ext)
		}
	}

	return nil
}

// getDefaultAllowedExtensions 获取默认允许的文件扩展名
func getDefaultAllowedExtensions() map[string]bool {
	return map[string]bool{
		// 图片文件
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
		".avif": true,
		".bmp":  true,
		".svg":  true,
		".ico":  true,
		// 文档文件
		".pdf":  true,
		".txt":  true,
		".doc":  true,
		".docx": true,
		".xls":  true,
		".xlsx": true,
		".ppt":  true,
		".pptx": true,
		// 压缩文件
		".zip": true,
		".rar": true,
		".7z":  true,
		".tar": true,
		".gz":  true,
		// 音视频文件
		".mp4":  true,
		".avi":  true,
		".mov":  true,
		".wmv":  true,
		".flv":  true,
		".webm": true,
		".mkv":  true,
		".m4v":  true,
		".mp3":  true,
		".wav":  true,
		".flac": true,
		".aac":  true,
	}
}

// isFileTypeAllowed 检查文件类型是否被允许（支持通配符）
func isFileTypeAllowed(filename string, allowedPatterns []string) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	// 获取文件的MIME类型
	mimeType := mime.TypeByExtension(ext)

	for _, pattern := range allowedPatterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))

		// 检查通配符模式
		if matchesPattern(ext, mimeType, pattern) {
			return true
		}
	}

	return false
}

// matchesPattern 检查文件是否匹配指定的模式
func matchesPattern(ext, mimeType, pattern string) bool {
	// 全通配符：允许所有文件
	if pattern == "*" {
		return true
	}

	// 扩展名精确匹配
	if pattern == ext {
		return true
	}

	// MIME类型通配符匹配
	if strings.Contains(pattern, "/") {
		// 处理 image/* 这样的模式
		if strings.HasSuffix(pattern, "/*") {
			mainType := strings.TrimSuffix(pattern, "/*")
			if mimeType != "" && strings.HasPrefix(mimeType, mainType+"/") {
				return true
			}
		}
		// 精确MIME类型匹配
		if pattern == mimeType {
			return true
		}
	}

	// 扩展名通配符匹配（如 *.jpg）
	if strings.HasPrefix(pattern, "*.") {
		targetExt := strings.TrimPrefix(pattern, "*")
		if targetExt == ext {
			return true
		}
	}

	// 文件名模式匹配（如 image.* 匹配 image.jpg, image.png 等）
	if strings.Contains(pattern, "*") {
		// 简单的通配符匹配
		return matchWildcard(ext, pattern)
	}

	return false
}

// matchWildcard 简单的通配符匹配实现
func matchWildcard(text, pattern string) bool {
	// 将模式转换为正则表达式风格的匹配
	if pattern == "*" {
		return true
	}

	// 处理 *.ext 格式
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(text, suffix)
	}

	// 处理 prefix.* 格式
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(text, prefix)
	}

	return false
}

// generateFilename 生成文件名
// 可以根据需要实现不同的命名策略
func generateFilename(originalFilename string, cfg *config.Config) string {
	// 获取文件扩展名
	ext := filepath.Ext(originalFilename)

	// 获取不带扩展名的文件名
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)

	// 清理文件名，移除特殊字符
	nameWithoutExt = sanitizeFilename(nameWithoutExt, cfg)
	
	// 生成时间戳前缀以避免文件名冲突
	timestamp := time.Now().Format("20060102_150405")
	
	// 组合最终文件名
	return fmt.Sprintf("%s_%s%s", timestamp, nameWithoutExt, ext)
}

// sanitizeFilename 清理文件名，移除或替换特殊字符
func sanitizeFilename(filename string, cfg *config.Config) string {
	// 替换空格为下划线
	filename = strings.ReplaceAll(filename, " ", "_")

	// 移除或替换其他特殊字符
	replacements := map[string]string{
		"/":  "_",
		"\\": "_",
		":":  "_",
		"*":  "_",
		"?":  "_",
		"\"": "_",
		"<":  "_",
		">":  "_",
		"|":  "_",
	}

	for old, new := range replacements {
		filename = strings.ReplaceAll(filename, old, new)
	}

	// 限制文件名长度（从配置读取，默认100）
	maxLength := cfg.Upload.MaxFilenameLength
	if maxLength <= 0 {
		maxLength = 100 // 默认值
	}
	if len(filename) > maxLength {
		filename = filename[:maxLength]
	}

	return filename
}

// HealthCheck 健康检查端点
func (h *UploadHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"service":   "file-uploader",
	})
}
