package storage

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"file_uploader/config"
)

// LocalStorage 本地存储实现
type LocalStorage struct {
	uploadDir string // 上传目录
	baseURL   string // 基础URL
}

// NewLocalStorage 创建本地存储实例
func NewLocalStorage(cfg *config.LocalConfig) *LocalStorage {
	// 确保上传目录存在
	os.MkdirAll(cfg.UploadDir, 0755)

	return &LocalStorage{
		uploadDir: cfg.UploadDir,
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
	}
}

// NewLocalStorageWithError 创建本地存储实例（带错误返回）
func NewLocalStorageWithError(cfg *config.LocalConfig) (*LocalStorage, error) {
	// 确保上传目录存在
	if err := os.MkdirAll(cfg.UploadDir, 0755); err != nil {
		return nil, NewStorageError("create_upload_dir", err)
	}

	return &LocalStorage{
		uploadDir: cfg.UploadDir,
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
	}, nil
}

// Upload 上传文件到本地存储
func (ls *LocalStorage) Upload(filename string, file multipart.File) (string, error) {
	// 构建完整的文件路径
	filePath := filepath.Join(ls.uploadDir, filename)

	// 确保目标目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", NewStorageError("create_dir", err)
	}

	// 创建目标文件
	dst, err := os.Create(filePath)
	if err != nil {
		return "", NewStorageError("create_file", err)
	}
	defer dst.Close()

	// 重置文件指针到开始位置
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", NewStorageError("seek_file", err)
	}

	// 复制文件内容
	if _, err := CopyFile(dst, file); err != nil {
		// 如果复制失败，删除已创建的文件
		os.Remove(filePath)
		return "", NewStorageError("copy_file", err)
	}

	// 返回文件访问URL
	url := ls.GetURL(filename)
	return url, nil
}

// UploadReader 从io.Reader上传文件到本地存储
func (ls *LocalStorage) UploadReader(filename string, reader io.Reader) (string, error) {
	// 构建完整的文件路径
	filePath := filepath.Join(ls.uploadDir, filename)

	// 确保目标目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", NewStorageError("create_dir", err)
	}

	// 创建目标文件
	dst, err := os.Create(filePath)
	if err != nil {
		return "", NewStorageError("create_file", err)
	}
	defer dst.Close()

	// 复制文件内容
	if _, err := CopyFile(dst, reader); err != nil {
		// 如果复制失败，删除已创建的文件
		os.Remove(filePath)
		return "", NewStorageError("copy_file", err)
	}

	// 返回文件访问URL
	url := ls.GetURL(filename)
	return url, nil
}

// Delete 删除本地文件
func (ls *LocalStorage) Delete(filename string) error {
	filePath := filepath.Join(ls.uploadDir, filename)
	
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return NewStorageError("file_not_found", err)
		}
		return NewStorageError("delete_file", err)
	}

	return nil
}

// Exists 检查文件是否存在
func (ls *LocalStorage) Exists(filename string) (bool, error) {
	filePath := filepath.Join(ls.uploadDir, filename)
	
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, NewStorageError("stat_file", err)
	}

	return true, nil
}

// GetURL 获取文件访问URL
func (ls *LocalStorage) GetURL(filename string) string {
	// 将Windows路径分隔符转换为URL路径分隔符
	urlPath := strings.ReplaceAll(filename, "\\", "/")

	// 确保文件名以正斜杠开头（用于URL路径）
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}

	return ls.baseURL + urlPath
}

// GetFileSize 获取文件大小
func (ls *LocalStorage) GetFileSize(filename string) (int64, error) {
	filePath := filepath.Join(ls.uploadDir, filename)
	
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, NewStorageError("file_not_found", err)
		}
		return 0, NewStorageError("stat_file", err)
	}

	return fileInfo.Size(), nil
}

// GetAbsolutePath 获取文件的绝对路径（内部使用）
func (ls *LocalStorage) GetAbsolutePath(filename string) string {
	return filepath.Join(ls.uploadDir, filename)
}

// ListFiles 列出上传目录中的所有文件（可选功能）
func (ls *LocalStorage) ListFiles() ([]string, error) {
	var files []string
	
	err := filepath.Walk(ls.uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() {
			// 获取相对于上传目录的路径
			relPath, err := filepath.Rel(ls.uploadDir, path)
			if err != nil {
				return err
			}
			// 将Windows路径分隔符转换为URL路径分隔符
			relPath = filepath.ToSlash(relPath)
			files = append(files, relPath)
		}
		
		return nil
	})
	
	if err != nil {
		return nil, NewStorageError("list_files", err)
	}
	
	return files, nil
}

// CleanupEmptyDirs 清理空目录（可选功能）
func (ls *LocalStorage) CleanupEmptyDirs() error {
	return filepath.Walk(ls.uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() && path != ls.uploadDir {
			// 尝试删除空目录
			if err := os.Remove(path); err != nil {
				// 如果目录不为空，忽略错误
				if !os.IsExist(err) {
					return nil
				}
			}
		}
		
		return nil
	})
}
