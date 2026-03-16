package storage

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"file_uploader/config"
)

// S3Storage S3存储实现
type S3Storage struct {
	client  *s3.S3
	bucket  string
	baseURL string
}

// NewS3Storage 创建S3存储实例
func NewS3Storage(cfg *config.S3Config) (*S3Storage, error) {
	// 创建AWS配置
	awsConfig := &aws.Config{
		Region: aws.String(cfg.Region),
		Credentials: credentials.NewStaticCredentials(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"", // token留空
		),
	}

	// 如果指定了自定义端点，使用自定义端点
	if cfg.Endpoint != "" {
		awsConfig.Endpoint = aws.String(cfg.Endpoint)
		awsConfig.S3ForcePathStyle = aws.Bool(true) // 对于自定义端点，通常需要路径样式
	}

	// 创建AWS会话
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, NewStorageError("create_session", err)
	}

	// 创建S3客户端
	client := s3.New(sess)

	// 验证存储桶是否存在
	if err := validateBucket(client, cfg.Bucket); err != nil {
		return nil, NewStorageError("validate_bucket", err)
	}

	return &S3Storage{
		client:  client,
		bucket:  cfg.Bucket,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
	}, nil
}

// Upload 上传文件到S3
func (s3s *S3Storage) Upload(filename string, file multipart.File) (string, error) {
	// 重置文件指针到开始位置
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", NewStorageError("seek_file", err)
	}

	// 读取文件内容到缓冲区
	buffer := bytes.NewBuffer(nil)
	if _, err := CopyFile(buffer, file); err != nil {
		return "", NewStorageError("read_file", err)
	}

	// 准备上传参数
	uploadInput := &s3.PutObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(filename),
		Body:   bytes.NewReader(buffer.Bytes()),
		ACL:    aws.String("public-read"), // 设置为公开读取
	}

	// 尝试检测文件类型
	if contentType := detectContentType(filename); contentType != "" {
		uploadInput.ContentType = aws.String(contentType)
	}

	// 执行上传
	_, err := s3s.client.PutObject(uploadInput)
	if err != nil {
		return "", NewStorageError("upload_file", err)
	}

	// 返回文件访问URL
	url := s3s.GetURL(filename)
	return url, nil
}

// UploadReader 从io.Reader上传文件到S3
func (s3s *S3Storage) UploadReader(filename string, reader io.Reader) (string, error) {
	// 读取文件内容到缓冲区
	buffer := bytes.NewBuffer(nil)
	if _, err := CopyFile(buffer, reader); err != nil {
		return "", NewStorageError("read_file", err)
	}

	// 准备上传参数
	uploadInput := &s3.PutObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(filename),
		Body:   bytes.NewReader(buffer.Bytes()),
		ACL:    aws.String("public-read"), // 设置为公开读取
	}

	// 尝试检测文件类型
	if contentType := detectContentType(filename); contentType != "" {
		uploadInput.ContentType = aws.String(contentType)
	}

	// 执行上传
	_, err := s3s.client.PutObject(uploadInput)
	if err != nil {
		return "", NewStorageError("upload_file", err)
	}

	// 返回文件访问URL
	url := s3s.GetURL(filename)
	return url, nil
}

// Delete 删除S3文件
func (s3s *S3Storage) Delete(filename string) error {
	deleteInput := &s3.DeleteObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(filename),
	}

	_, err := s3s.client.DeleteObject(deleteInput)
	if err != nil {
		return NewStorageError("delete_file", err)
	}

	return nil
}

// Exists 检查S3文件是否存在
func (s3s *S3Storage) Exists(filename string) (bool, error) {
	headInput := &s3.HeadObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(filename),
	}

	_, err := s3s.client.HeadObject(headInput)
	if err != nil {
		// 检查是否是"文件不存在"错误
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, NewStorageError("head_object", err)
	}

	return true, nil
}

// GetURL 获取S3文件访问URL
func (s3s *S3Storage) GetURL(filename string) string {
	// 确保文件名以正斜杠开头（用于URL路径）
	urlPath := filename
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	
	return s3s.baseURL + urlPath
}

// GetFileSize 获取S3文件大小
func (s3s *S3Storage) GetFileSize(filename string) (int64, error) {
	headInput := &s3.HeadObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(filename),
	}

	result, err := s3s.client.HeadObject(headInput)
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return 0, NewStorageError("file_not_found", err)
		}
		return 0, NewStorageError("head_object", err)
	}

	if result.ContentLength == nil {
		return 0, NewStorageError("get_content_length", fmt.Errorf("content length is nil"))
	}

	return *result.ContentLength, nil
}

// validateBucket 验证存储桶是否存在且可访问
func validateBucket(client *s3.S3, bucket string) error {
	headInput := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}

	_, err := client.HeadBucket(headInput)
	if err != nil {
		// 提供更友好的错误信息
		if strings.Contains(err.Error(), "Forbidden") {
			return fmt.Errorf("S3存储桶 '%s' 访问被拒绝，请检查访问密钥权限", bucket)
		} else if strings.Contains(err.Error(), "NoSuchBucket") {
			return fmt.Errorf("S3存储桶 '%s' 不存在，请检查桶名称", bucket)
		} else if strings.Contains(err.Error(), "InvalidAccessKeyId") {
			return fmt.Errorf("S3访问密钥无效，请检查 access_key_id 配置")
		} else if strings.Contains(err.Error(), "SignatureDoesNotMatch") {
			return fmt.Errorf("S3密钥签名错误，请检查 secret_access_key 配置")
		} else {
			return fmt.Errorf("S3存储桶 '%s' 连接失败: %v", bucket, err)
		}
	}

	return nil
}

// detectContentType 根据文件扩展名检测内容类型
func detectContentType(filename string) string {
	ext := strings.ToLower(filename[strings.LastIndex(filename, ".")+1:])
	
	contentTypes := map[string]string{
		"jpg":  "image/jpeg",
		"jpeg": "image/jpeg",
		"png":  "image/png",
		"gif":  "image/gif",
		"webp": "image/webp",
		"pdf":  "application/pdf",
		"txt":  "text/plain",
		"html": "text/html",
		"css":  "text/css",
		"js":   "application/javascript",
		"json": "application/json",
		"xml":  "application/xml",
		"zip":  "application/zip",
		"mp4":  "video/mp4",
		"mp3":  "audio/mpeg",
		"wav":  "audio/wav",
	}

	if contentType, exists := contentTypes[ext]; exists {
		return contentType
	}

	return "application/octet-stream" // 默认二进制类型
}

// ListObjects 列出S3存储桶中的对象（可选功能）
func (s3s *S3Storage) ListObjects(prefix string) ([]string, error) {
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(s3s.bucket),
	}

	if prefix != "" {
		listInput.Prefix = aws.String(prefix)
	}

	var objects []string
	err := s3s.client.ListObjectsV2Pages(listInput, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, obj := range page.Contents {
			if obj.Key != nil {
				objects = append(objects, *obj.Key)
			}
		}
		return !lastPage
	})

	if err != nil {
		return nil, NewStorageError("list_objects", err)
	}

	return objects, nil
}
