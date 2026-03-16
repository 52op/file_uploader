package image

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/gen2brain/avif"
	"github.com/nfnt/resize"

	"file_uploader/config"
	"file_uploader/metrics"
	"file_uploader/stats"
)

// ImageProcessor 图片处理器
type ImageProcessor struct {
	config *config.Config
}

// NewImageProcessor 创建图片处理器
func NewImageProcessor(cfg *config.Config) *ImageProcessor {
	return &ImageProcessor{
		config: cfg,
	}
}

// ThumbnailConfig 缩略图配置
type ThumbnailConfig struct {
	Width     uint   // 缩略图宽度
	Height    uint   // 缩略图高度
	Quality   int    // JPEG质量 (1-100)
	Suffix    string // 文件名后缀
	MinWidth  int    // 原图最小宽度（小于此值不生成缩略图）
	MinHeight int    // 原图最小高度（小于此值不生成缩略图）
	MinSize   int64  // 原图最小文件大小（字节，小于此值不生成缩略图）
}

// GetThumbnailConfigFromConfig 从配置文件获取缩略图配置
func GetThumbnailConfigFromConfig(cfg *config.Config) ThumbnailConfig {
	if cfg == nil || !cfg.Thumbnail.Enabled {
		// 如果配置为空或禁用缩略图，返回禁用的配置
		return ThumbnailConfig{
			Width:     0,
			Height:    0,
			Quality:   0,
			Suffix:    "_Thumbnail",
			MinWidth:  999999, // 设置一个很大的值，确保不会生成缩略图
			MinHeight: 999999,
			MinSize:   999999999,
		}
	}

	return ThumbnailConfig{
		Width:     uint(cfg.Thumbnail.Width),
		Height:    uint(cfg.Thumbnail.Height),
		Quality:   cfg.Thumbnail.Quality,
		Suffix:    "_Thumbnail",
		MinWidth:  cfg.Thumbnail.MinWidth,
		MinHeight: cfg.Thumbnail.MinHeight,
		MinSize:   int64(cfg.Thumbnail.MinSizeKB) * 1024, // 转换为字节
	}
}

// GetDefaultThumbnailConfig 获取默认缩略图配置（向后兼容）
func GetDefaultThumbnailConfig() ThumbnailConfig {
	return ThumbnailConfig{
		Width:     200,
		Height:    200,
		Quality:   85,
		Suffix:    "_Thumbnail",
		MinWidth:  400,  // 原图宽度至少400px才生成缩略图
		MinHeight: 400,  // 原图高度至少400px才生成缩略图
		MinSize:   50 * 1024, // 原图文件大小至少50KB才生成缩略图
	}
}

// IsImageFile 检查文件是否为图片
func IsImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	supportedExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif"}

	for _, supportedExt := range supportedExts {
		if ext == supportedExt {
			return true
		}
	}
	return false
}

// GenerateThumbnail 生成缩略图
func (p *ImageProcessor) GenerateThumbnail(originalFile multipart.File, originalFilename string, fileSize int64, config ThumbnailConfig) (io.Reader, string, error) {
	start := time.Now()
	m := metrics.GetMetrics()
	statsCollector := stats.GetStats()

	defer func() {
		duration := time.Since(start)
		// 这里会在函数返回时根据是否有错误来记录指标
		if recover() != nil {
			m.RecordThumbnail(duration, false)
			statsCollector.RecordThumbnail(false)
		}
	}()

	// 检查是否为图片文件
	if !IsImageFile(originalFilename) {
		return nil, "", fmt.Errorf("不支持的图片格式: %s", filepath.Ext(originalFilename))
	}

	// 检查文件大小是否满足生成缩略图的条件
	if fileSize < config.MinSize {
		return nil, "", fmt.Errorf("文件大小 %d 字节小于最小要求 %d 字节，跳过缩略图生成", fileSize, config.MinSize)
	}

	// 重置文件指针到开始位置
	if seeker, ok := originalFile.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// 先获取图片配置信息检查尺寸
	imgConfig, _, err := image.DecodeConfig(originalFile)
	if err != nil {
		m.RecordThumbnail(time.Since(start), false)
		statsCollector.RecordThumbnail(false)
		return nil, "", fmt.Errorf("解码图片配置失败: %v", err)
	}

	// 检查图片尺寸是否满足生成缩略图的条件
	if imgConfig.Width < config.MinWidth || imgConfig.Height < config.MinHeight {
		return nil, "", fmt.Errorf("图片尺寸 %dx%d 小于最小要求 %dx%d，跳过缩略图生成",
			imgConfig.Width, imgConfig.Height, config.MinWidth, config.MinHeight)
	}

	// 重置文件指针到开始位置
	if seeker, ok := originalFile.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// 解码图片
	img, format, err := image.Decode(originalFile)
	if err != nil {
		m.RecordThumbnail(time.Since(start), false)
		statsCollector.RecordThumbnail(false)
		return nil, "", fmt.Errorf("解码图片失败: %v", err)
	}

	// 生成缩略图
	thumbnail := resize.Thumbnail(config.Width, config.Height, img, resize.Lanczos3)

	// 生成缩略图文件名
	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)
	thumbnailFilename := nameWithoutExt + config.Suffix + ext

	// 创建缓冲区来存储缩略图数据
	var buf strings.Builder
	writer := &stringWriter{&buf}

	// 根据原始格式编码缩略图
	switch format {
	case "jpeg":
		err = jpeg.Encode(writer, thumbnail, &jpeg.Options{Quality: config.Quality})
	case "png":
		err = png.Encode(writer, thumbnail)
	case "gif":
		err = gif.Encode(writer, thumbnail, nil)
	case "webp":
		// WebP编码需要特殊处理，这里转换为JPEG
		err = jpeg.Encode(writer, thumbnail, &jpeg.Options{Quality: config.Quality})
		// 更新文件扩展名为jpg
		thumbnailFilename = nameWithoutExt + config.Suffix + ".jpg"
	case "avif":
		// AVIF编码需要特殊处理，这里转换为JPEG
		err = jpeg.Encode(writer, thumbnail, &jpeg.Options{Quality: config.Quality})
		// 更新文件扩展名为jpg
		thumbnailFilename = nameWithoutExt + config.Suffix + ".jpg"
	default:
		// 默认使用JPEG格式
		err = jpeg.Encode(writer, thumbnail, &jpeg.Options{Quality: config.Quality})
		thumbnailFilename = nameWithoutExt + config.Suffix + ".jpg"
	}

	if err != nil {
		m.RecordThumbnail(time.Since(start), false)
		statsCollector.RecordThumbnail(false)
		return nil, "", fmt.Errorf("编码缩略图失败: %v", err)
	}

	// 记录成功生成缩略图
	duration := time.Since(start)
	m.RecordThumbnail(duration, true)
	statsCollector.RecordThumbnail(true)

	// 返回缩略图数据和文件名
	return strings.NewReader(buf.String()), thumbnailFilename, nil
}

// stringWriter 实现io.Writer接口，用于写入字符串
type stringWriter struct {
	builder *strings.Builder
}

func (sw *stringWriter) Write(p []byte) (n int, err error) {
	return sw.builder.Write(p)
}

// ProcessImageUpload 处理图片上传，包括生成缩略图
func (p *ImageProcessor) ProcessImageUpload(file multipart.File, filename string, fileSize int64, cfg *config.Config) (*ImageUploadResult, error) {
	result := &ImageUploadResult{
		OriginalFilename: filename,
		IsImage:         IsImageFile(filename),
	}

	// 如果不是图片文件，直接返回
	if !result.IsImage {
		return result, nil
	}

	// 从配置获取缩略图配置
	thumbnailConfig := GetThumbnailConfigFromConfig(cfg)

	// 如果缩略图被禁用，直接返回
	if !cfg.Thumbnail.Enabled {
		return result, nil
	}

	thumbnailReader, thumbnailFilename, err := p.GenerateThumbnail(file, filename, fileSize, thumbnailConfig)
	if err != nil {
		// 缩略图生成失败不影响原图上传，但设置使用原图作为缩略图
		result.ThumbnailError = err.Error()
		result.UseOriginalAsThumbnail = true // 标记使用原图作为缩略图
		log.Printf("缩略图生成跳过: %s - %v", filename, err)
		return result, nil
	}

	result.HasThumbnail = true
	result.ThumbnailFilename = thumbnailFilename
	result.ThumbnailReader = thumbnailReader

	return result, nil
}

// ImageUploadResult 图片上传结果
type ImageUploadResult struct {
	OriginalFilename        string    // 原始文件名
	IsImage                 bool      // 是否为图片
	HasThumbnail            bool      // 是否有缩略图
	ThumbnailFilename       string    // 缩略图文件名
	ThumbnailReader         io.Reader // 缩略图数据读取器
	ThumbnailError          string    // 缩略图生成错误信息
	UseOriginalAsThumbnail  bool      // 是否使用原图作为缩略图（当不满足生成条件时）
}

// GetImageInfo 获取图片信息
func GetImageInfo(file multipart.File) (*ImageInfo, error) {
	// 重置文件指针
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// 解码图片获取基本信息
	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("解码图片配置失败: %v", err)
	}

	return &ImageInfo{
		Width:  config.Width,
		Height: config.Height,
		Format: format,
	}, nil
}

// ImageInfo 图片信息
type ImageInfo struct {
	Width  int    // 图片宽度
	Height int    // 图片高度
	Format string // 图片格式
}

// ValidateImageFile 验证图片文件
func ValidateImageFile(file multipart.File, filename string) error {
	// 检查文件扩展名
	if !IsImageFile(filename) {
		return fmt.Errorf("不支持的图片格式")
	}

	// 尝试解码图片以验证文件完整性
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	_, _, err := image.DecodeConfig(file)
	if err != nil {
		return fmt.Errorf("图片文件损坏或格式不正确: %v", err)
	}

	// 重置文件指针
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	return nil
}

// GetThumbnailFilename 根据原始文件名生成缩略图文件名
func GetThumbnailFilename(originalFilename string) string {
	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)
	return nameWithoutExt + "_Thumbnail" + ext
}

// IsThumbnailFile 检查文件名是否为缩略图文件
func IsThumbnailFile(filename string) bool {
	return strings.Contains(filename, "_Thumbnail")
}

// GetOriginalFilename 从缩略图文件名获取原始文件名
func GetOriginalFilename(thumbnailFilename string) string {
	return strings.Replace(thumbnailFilename, "_Thumbnail", "", 1)
}

// init 注册AVIF解码器
func init() {
	// 注册AVIF格式解码器
	image.RegisterFormat("avif", "????ftypavif", avif.Decode, avif.DecodeConfig)
}
