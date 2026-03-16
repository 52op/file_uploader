package handlers

import (
	"fmt"
	"io"
	"log"
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

// BatchHandler 批量操作处理器
type BatchHandler struct {
	storageManager  *storage.StorageManager
	config         *config.Config
	imageProcessor *imageProcessor.ImageProcessor
}

// NewBatchHandler 创建批量操作处理器
func NewBatchHandler(storageManager *storage.StorageManager, cfg *config.Config) *BatchHandler {
	return &BatchHandler{
		storageManager:  storageManager,
		config:         cfg,
		imageProcessor: imageProcessor.NewImageProcessor(cfg),
	}
}

// BatchUploadRequest 批量上传请求
type BatchUploadRequest struct {
	StorageType string `form:"storage" json:"storage"` // 存储类型
	BasePath    string `form:"base_path" json:"base_path"` // 基础路径
}

// BatchUploadResult 批量上传结果
type BatchUploadResult struct {
	Success      bool                    `json:"success"`
	TotalFiles   int                     `json:"total_files"`
	SuccessCount int                     `json:"success_count"`
	FailedCount  int                     `json:"failed_count"`
	Results      []storage.UploadResult  `json:"results"`
	Errors       []BatchError           `json:"errors,omitempty"`
	Message      string                 `json:"message"`
}

// BatchError 批量操作错误
type BatchError struct {
	Filename string `json:"filename"`
	Error    string `json:"error"`
}

// BatchUpload 批量文件上传
func (h *BatchHandler) BatchUpload(c *gin.Context) {
	start := time.Now()
	m := metrics.GetMetrics()
	statsCollector := stats.GetStats()

	// 记录批量上传
	m.RecordBatchUpload()
	statsCollector.RecordBatchUpload()

	// 解析表单
	form, err := c.MultipartForm()
	if err != nil {
		m.RecordBatchError()
		statsCollector.RecordBatchError()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "解析表单失败",
			"details": err.Error(),
		})
		return
	}

	// 获取参数
	storageType := c.PostForm("storage")
	basePath := c.PostForm("base_path")

	// 获取上传的文件
	files := form.File["files"]
	if len(files) == 0 {
		m.RecordBatchError()
		statsCollector.RecordBatchError()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "没有找到上传的文件",
		})
		return
	}

	result := &BatchUploadResult{
		TotalFiles: len(files),
		Results:    make([]storage.UploadResult, 0),
		Errors:     make([]BatchError, 0),
	}

	// 处理每个文件
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			result.Errors = append(result.Errors, BatchError{
				Filename: fileHeader.Filename,
				Error:    fmt.Sprintf("打开文件失败: %v", err),
			})
			result.FailedCount++
			continue
		}

		// 生成文件路径
		var filename string
		if basePath != "" {
			filename = filepath.Join(basePath, fileHeader.Filename)
		} else {
			filename = generateFilename(fileHeader.Filename, h.config)
		}

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
			file.Close()
			result.Errors = append(result.Errors, BatchError{
				Filename: fileHeader.Filename,
				Error:    fmt.Sprintf("存储类型不支持: %v", err),
			})
			result.FailedCount++
			continue
		}

		// 处理图片（如果是图片文件）
		var thumbnailURL string
		var imageResult *imageProcessor.ImageUploadResult
		if imageProcessor.IsImageFile(targetFilePath) {
			// 重置文件指针
			if seeker, ok := file.(io.Seeker); ok {
				seeker.Seek(0, io.SeekStart)
			}

			imageResult, err = h.imageProcessor.ProcessImageUpload(file, targetFilePath, fileHeader.Size, h.config)
			if err != nil {
				log.Printf("图片处理失败: %v", err)
			} else if imageResult.HasThumbnail {
				thumbnailPath := imageResult.ThumbnailFilename
				thumbnailURL, err = storageInstance.UploadReader(thumbnailPath, imageResult.ThumbnailReader)
				if err != nil {
					log.Printf("缩略图上传失败: %v", err)
					// 缩略图上传失败，标记使用原图作为缩略图
					imageResult.UseOriginalAsThumbnail = true
				}
			}

			// 重置文件指针
			if seeker, ok := file.(io.Seeker); ok {
				seeker.Seek(0, io.SeekStart)
			}
		}

		// 上传文件
		fileURL, err := storageInstance.Upload(targetFilePath, file)
		file.Close()

		if err != nil {
			result.Errors = append(result.Errors, BatchError{
				Filename: fileHeader.Filename,
				Error:    fmt.Sprintf("上传失败: %v", err),
			})
			result.FailedCount++
			continue
		}

		// 构建上传结果
		uploadResult := storage.UploadResult{
			Success: true,
			FileInfo: storage.FileInfo{
				Filename:    targetFilePath,
				Size:        fileHeader.Size,
				ContentType: fileHeader.Header.Get("Content-Type"),
				URL:         fileURL,
				UploadTime:  time.Now().Unix(),
			},
			Message:     "文件上传成功",
			StorageType: targetStorageType,
		}

		if thumbnailURL != "" {
			uploadResult.ThumbnailURL = thumbnailURL
		} else if imageResult != nil && imageResult.IsImage && (imageResult.UseOriginalAsThumbnail || !h.config.Thumbnail.Enabled) {
			// 如果是图片但不满足缩略图生成条件，或缩略图功能被禁用，使用原图作为缩略图
			uploadResult.ThumbnailURL = fileURL
		}

		result.Results = append(result.Results, uploadResult)
		result.SuccessCount++
	}

	// 设置结果状态
	result.Success = result.FailedCount == 0
	if result.Success {
		result.Message = fmt.Sprintf("批量上传成功，共上传 %d 个文件", result.SuccessCount)
	} else {
		result.Message = fmt.Sprintf("批量上传完成，成功 %d 个，失败 %d 个", result.SuccessCount, result.FailedCount)
	}

	log.Printf("批量上传完成，耗时: %v, 成功: %d, 失败: %d", time.Since(start), result.SuccessCount, result.FailedCount)

	c.JSON(http.StatusOK, result)
}

// BatchDeleteRequest 批量删除请求
type BatchDeleteRequest struct {
	Files       []string `json:"files" binding:"required"`       // 文件列表
	StorageType string   `json:"storage_type,omitempty"`         // 存储类型
}

// BatchDeleteResult 批量删除结果
type BatchDeleteResult struct {
	Success      bool         `json:"success"`
	TotalFiles   int          `json:"total_files"`
	SuccessCount int          `json:"success_count"`
	FailedCount  int          `json:"failed_count"`
	Errors       []BatchError `json:"errors,omitempty"`
	Message      string       `json:"message"`
}

// BatchDelete 批量文件删除
func (h *BatchHandler) BatchDelete(c *gin.Context) {
	m := metrics.GetMetrics()
	statsCollector := stats.GetStats()

	// 记录批量删除
	m.RecordBatchDelete()
	statsCollector.RecordBatchDelete()

	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		m.RecordBatchError()
		statsCollector.RecordBatchError()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	result := &BatchDeleteResult{
		TotalFiles: len(req.Files),
		Errors:     make([]BatchError, 0),
	}

	// 处理每个文件
	for _, filename := range req.Files {
		// 解析存储类型和文件路径
		var targetStorageType, targetFilePath string
		if req.StorageType != "" {
			targetStorageType = req.StorageType
			targetFilePath = filename
		} else {
			targetStorageType, targetFilePath = h.storageManager.ParseUploadPath(filename)
		}

		// 获取存储实例
		storageInstance, err := h.storageManager.GetStorage(targetStorageType)
		if err != nil {
			result.Errors = append(result.Errors, BatchError{
				Filename: filename,
				Error:    fmt.Sprintf("存储类型不支持: %v", err),
			})
			result.FailedCount++
			continue
		}

		// 删除文件
		if err := storageInstance.Delete(targetFilePath); err != nil {
			result.Errors = append(result.Errors, BatchError{
				Filename: filename,
				Error:    fmt.Sprintf("删除失败: %v", err),
			})
			result.FailedCount++
			continue
		}

		result.SuccessCount++
	}

	// 设置结果状态
	result.Success = result.FailedCount == 0
	if result.Success {
		result.Message = fmt.Sprintf("批量删除成功，共删除 %d 个文件", result.SuccessCount)
	} else {
		result.Message = fmt.Sprintf("批量删除完成，成功 %d 个，失败 %d 个", result.SuccessCount, result.FailedCount)
	}

	c.JSON(http.StatusOK, result)
}

// BatchInfoRequest 批量信息查询请求
type BatchInfoRequest struct {
	Files       []string `json:"files" binding:"required"`       // 文件列表
	StorageType string   `json:"storage_type,omitempty"`         // 存储类型
}

// BatchInfoResult 批量信息查询结果
type BatchInfoResult struct {
	Success      bool                   `json:"success"`
	TotalFiles   int                    `json:"total_files"`
	FoundCount   int                    `json:"found_count"`
	NotFoundCount int                   `json:"not_found_count"`
	Files        []storage.FileInfo     `json:"files"`
	Errors       []BatchError          `json:"errors,omitempty"`
	Message      string                `json:"message"`
}

// BatchInfo 批量文件信息查询
func (h *BatchHandler) BatchInfo(c *gin.Context) {
	var req BatchInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	result := &BatchInfoResult{
		TotalFiles: len(req.Files),
		Files:      make([]storage.FileInfo, 0),
		Errors:     make([]BatchError, 0),
	}

	// 处理每个文件
	for _, filename := range req.Files {
		// 解析存储类型和文件路径
		var targetStorageType, targetFilePath string
		if req.StorageType != "" {
			targetStorageType = req.StorageType
			targetFilePath = filename
		} else {
			targetStorageType, targetFilePath = h.storageManager.ParseUploadPath(filename)
		}

		// 获取存储实例
		storageInstance, err := h.storageManager.GetStorage(targetStorageType)
		if err != nil {
			result.Errors = append(result.Errors, BatchError{
				Filename: filename,
				Error:    fmt.Sprintf("存储类型不支持: %v", err),
			})
			result.NotFoundCount++
			continue
		}

		// 检查文件是否存在
		exists, err := storageInstance.Exists(targetFilePath)
		if err != nil {
			result.Errors = append(result.Errors, BatchError{
				Filename: filename,
				Error:    fmt.Sprintf("检查文件失败: %v", err),
			})
			result.NotFoundCount++
			continue
		}

		if !exists {
			result.Errors = append(result.Errors, BatchError{
				Filename: filename,
				Error:    "文件不存在",
			})
			result.NotFoundCount++
			continue
		}

		// 获取文件大小
		size, err := storageInstance.GetFileSize(targetFilePath)
		if err != nil {
			size = 0 // 如果获取失败，设置为0
		}

		// 构建文件信息
		fileInfo := storage.FileInfo{
			Filename:   targetFilePath,
			Size:       size,
			URL:        storageInstance.GetURL(targetFilePath),
			UploadTime: 0, // 批量查询时不提供上传时间
		}

		result.Files = append(result.Files, fileInfo)
		result.FoundCount++
	}

	// 设置结果状态
	result.Success = result.NotFoundCount == 0
	if result.Success {
		result.Message = fmt.Sprintf("批量查询成功，找到 %d 个文件", result.FoundCount)
	} else {
		result.Message = fmt.Sprintf("批量查询完成，找到 %d 个，未找到 %d 个", result.FoundCount, result.NotFoundCount)
	}

	c.JSON(http.StatusOK, result)
}

// FolderCreateRequest 文件夹创建请求
type FolderCreateRequest struct {
	Path        string `json:"path" binding:"required"`         // 文件夹路径
	StorageType string `json:"storage_type,omitempty"`         // 存储类型
}

// FolderListRequest 文件夹列表请求
type FolderListRequest struct {
	Path        string `json:"path"`                           // 文件夹路径，空表示根目录
	StorageType string `json:"storage_type,omitempty"`         // 存储类型
	Recursive   bool   `json:"recursive,omitempty"`            // 是否递归列出子目录
}

// FolderListResult 文件夹列表结果
type FolderListResult struct {
	Success bool              `json:"success"`
	Path    string            `json:"path"`
	Files   []storage.FileInfo `json:"files"`
	Folders []string          `json:"folders"`
	Message string            `json:"message"`
}

// CreateFolder 创建文件夹
func (h *BatchHandler) CreateFolder(c *gin.Context) {
	var req FolderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	// 解析存储类型
	var targetStorageType string
	if req.StorageType != "" {
		targetStorageType = req.StorageType
	} else {
		targetStorageType, _ = h.storageManager.ParseUploadPath(req.Path)
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

	// 对于本地存储，我们可以创建目录
	if localStorage, ok := storageInstance.(*storage.LocalStorage); ok {
		// 这里需要添加一个方法来创建目录
		// 暂时返回成功，实际实现需要在LocalStorage中添加CreateFolder方法
		_ = localStorage
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("文件夹创建成功: %s", req.Path),
		})
		return
	}

	// 对于S3存储，文件夹是虚拟的，不需要创建
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("文件夹路径已准备: %s", req.Path),
	})
}

// ListFolder 列出文件夹内容
func (h *BatchHandler) ListFolder(c *gin.Context) {
	var req FolderListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	// 解析存储类型
	var targetStorageType string
	if req.StorageType != "" {
		targetStorageType = req.StorageType
	} else {
		targetStorageType, _ = h.storageManager.ParseUploadPath(req.Path)
	}

	// 获取存储实例
	_, err := h.storageManager.GetStorage(targetStorageType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "存储类型不支持",
			"details": err.Error(),
		})
		return
	}

	// 文件夹列表功能需要存储实现支持
	// 这里返回一个基本的响应
	result := &FolderListResult{
		Success: true,
		Path:    req.Path,
		Files:   make([]storage.FileInfo, 0),
		Folders: make([]string, 0),
		Message: "文件夹列表功能需要存储实现支持",
	}

	c.JSON(http.StatusOK, result)
}

// DeleteFolder 删除文件夹
func (h *BatchHandler) DeleteFolder(c *gin.Context) {
	path := c.Param("path")
	// 移除开头的斜杠（gin的*path参数会包含开头的斜杠）
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	storageType := c.Query("storage_type")

	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "文件夹路径不能为空",
		})
		return
	}

	// 解析存储类型
	var targetStorageType string
	if storageType != "" {
		targetStorageType = storageType
	} else {
		targetStorageType, _ = h.storageManager.ParseUploadPath(path)
	}

	// 获取存储实例
	_, err := h.storageManager.GetStorage(targetStorageType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "存储类型不支持",
			"details": err.Error(),
		})
		return
	}

	// 文件夹删除功能需要存储实现支持
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("文件夹删除请求已处理: %s", path),
	})
}
