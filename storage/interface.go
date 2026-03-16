package storage

import (
	"io"
	"mime/multipart"
)

// Storage 存储接口定义
// 提供统一的文件存储操作接口，支持本地存储和S3存储
type Storage interface {
	// Upload 上传文件
	// filename: 文件名
	// file: 文件内容
	// 返回: 文件访问URL和错误信息
	Upload(filename string, file multipart.File) (string, error)

	// UploadReader 上传文件（从io.Reader）
	// filename: 文件名
	// reader: 文件内容读取器
	// 返回: 文件访问URL和错误信息
	UploadReader(filename string, reader io.Reader) (string, error)

	// Delete 删除文件
	// filename: 文件名
	// 返回: 错误信息
	Delete(filename string) error

	// Exists 检查文件是否存在
	// filename: 文件名
	// 返回: 是否存在和错误信息
	Exists(filename string) (bool, error)

	// GetURL 获取文件访问URL
	// filename: 文件名
	// 返回: 文件访问URL
	GetURL(filename string) string

	// GetFileSize 获取文件大小
	// filename: 文件名
	// 返回: 文件大小（字节）和错误信息
	GetFileSize(filename string) (int64, error)
}

// FileInfo 文件信息结构
type FileInfo struct {
	Filename    string `json:"filename"`     // 文件名
	Size        int64  `json:"size"`         // 文件大小（字节）
	ContentType string `json:"content_type"` // 文件MIME类型
	URL         string `json:"url"`          // 文件访问URL
	UploadTime  int64  `json:"upload_time"`  // 上传时间戳
}

// StorageSecurityConfig 存储安全配置
type StorageSecurityConfig struct {
	RequireAuth   *bool    `yaml:"require_auth"`   // 是否需要签名验证（nil表示使用全局默认）
	AllowReferer  []string `yaml:"allow_referer"`  // 允许的Referer列表，空表示不检查
}

// UploadResult 上传结果结构
type UploadResult struct {
	Success      bool   `json:"success"`                  // 上传是否成功
	FileInfo     FileInfo `json:"file_info"`              // 文件信息
	Message      string `json:"message,omitempty"`        // 消息（通常用于错误信息）
	StorageType  string `json:"storage_type,omitempty"`   // 使用的存储类型
	ThumbnailURL string `json:"thumbnail_url,omitempty"`  // 缩略图URL（如果是图片文件）
}

// StorageError 存储错误类型
type StorageError struct {
	Op  string // 操作名称
	Err error  // 原始错误
}

func (e *StorageError) Error() string {
	return "storage " + e.Op + ": " + e.Err.Error()
}

// NewStorageError 创建存储错误
func NewStorageError(op string, err error) *StorageError {
	return &StorageError{
		Op:  op,
		Err: err,
	}
}

// CopyFile 复制文件内容的辅助函数
func CopyFile(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}
