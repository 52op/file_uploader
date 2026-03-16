package image

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/gen2brain/avif"
	"github.com/gin-gonic/gin"

	"file_uploader/config"
)

// OptimizeParams 图片优化参数
type OptimizeParams struct {
	Width   int    // 宽度
	Height  int    // 高度
	Quality int    // 质量 (1-100)
	Format  string // 格式: jpeg, png, webp, avif
}

// ImageOptimizer 图片优化器
type ImageOptimizer struct {
	config *config.Config
}

// NewImageOptimizer 创建图片优化器
func NewImageOptimizer(cfg *config.Config) *ImageOptimizer {
	return &ImageOptimizer{
		config: cfg,
	}
}

// ParseOptimizeParams 从查询参数解析优化参数
func (opt *ImageOptimizer) ParseOptimizeParams(c *gin.Context) *OptimizeParams {
	params := &OptimizeParams{
		Quality: opt.config.ImageOptimize.DefaultQuality,
	}

	// 解析宽度
	if w := c.Query("w"); w != "" {
		if width, err := strconv.Atoi(w); err == nil && width > 0 {
			params.Width = width
		}
	}

	// 解析高度
	if h := c.Query("h"); h != "" {
		if height, err := strconv.Atoi(h); err == nil && height > 0 {
			params.Height = height
		}
	}

	// 解析质量
	if q := c.Query("q"); q != "" {
		if quality, err := strconv.Atoi(q); err == nil && quality >= 1 && quality <= 100 {
			params.Quality = quality
		}
	}

	// 解析格式
	if f := c.Query("f"); f != "" {
		format := strings.ToLower(f)
		if opt.isFormatAllowed(format) {
			params.Format = format
		}
	}

	return params
}

// isFormatAllowed 检查格式是否被允许
func (opt *ImageOptimizer) isFormatAllowed(format string) bool {
	for _, allowed := range opt.config.ImageOptimize.AllowedFormats {
		if strings.ToLower(allowed) == format {
			return true
		}
	}
	return false
}

// ShouldOptimize 判断是否需要优化
func (opt *ImageOptimizer) ShouldOptimize(params *OptimizeParams, filePath string) bool {
	if !opt.config.ImageOptimize.Enabled {
		return false
	}

	// 检查是否是图片文件
	if !opt.isImageFile(filePath) {
		return false
	}

	// 如果有任何优化参数，则需要优化
	return params.Width > 0 || params.Height > 0 || params.Quality != opt.config.ImageOptimize.DefaultQuality || params.Format != ""
}

// isImageFile 检查是否是图片文件
func (opt *ImageOptimizer) isImageFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".avif"}

	for _, imgExt := range imageExts {
		if ext == imgExt {
			return true
		}
	}
	return false
}

// OptimizeImage 优化图片
func (opt *ImageOptimizer) OptimizeImage(src io.Reader, params *OptimizeParams) ([]byte, string, error) {
	// 解码图片
	img, format, err := image.Decode(src)
	if err != nil {
		return nil, "", fmt.Errorf("解码图片失败: %v", err)
	}

	log.Printf("[图片优化] 原始图片: %dx%d, 格式: %s", img.Bounds().Dx(), img.Bounds().Dy(), format)

	// 应用尺寸限制
	params = opt.applyLimits(params)

	// 调整尺寸
	if params.Width > 0 || params.Height > 0 {
		img = opt.resizeImage(img, params.Width, params.Height)
		log.Printf("[图片优化] 调整尺寸后: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// 确定输出格式
	outputFormat := params.Format
	if outputFormat == "" {
		outputFormat = format
	}

	// 编码图片
	data, contentType, err := opt.encodeImage(img, outputFormat, params.Quality)
	if err != nil {
		return nil, "", fmt.Errorf("编码图片失败: %v", err)
	}

	log.Printf("[图片优化] 输出格式: %s, 质量: %d, 大小: %d bytes", outputFormat, params.Quality, len(data))

	return data, contentType, nil
}

// applyLimits 应用尺寸限制
func (opt *ImageOptimizer) applyLimits(params *OptimizeParams) *OptimizeParams {
	result := *params

	// 应用最大宽度限制
	if opt.config.ImageOptimize.MaxWidth > 0 && result.Width > opt.config.ImageOptimize.MaxWidth {
		result.Width = opt.config.ImageOptimize.MaxWidth
	}

	// 应用最大高度限制
	if opt.config.ImageOptimize.MaxHeight > 0 && result.Height > opt.config.ImageOptimize.MaxHeight {
		result.Height = opt.config.ImageOptimize.MaxHeight
	}

	return &result
}

// resizeImage 调整图片尺寸
func (opt *ImageOptimizer) resizeImage(img image.Image, width, height int) image.Image {
	if width == 0 && height == 0 {
		return img
	}

	// 如果只指定了一个维度，保持宽高比
	if width == 0 {
		return imaging.Resize(img, 0, height, imaging.Lanczos)
	}
	if height == 0 {
		return imaging.Resize(img, width, 0, imaging.Lanczos)
	}

	// 如果指定了两个维度，使用Fit模式保持宽高比
	return imaging.Fit(img, width, height, imaging.Lanczos)
}

// encodeImage 编码图片
func (opt *ImageOptimizer) encodeImage(img image.Image, format string, quality int) ([]byte, string, error) {
	buf := make([]byte, 0, 1024*1024) // 预分配1MB
	writer := &bytesWriter{data: buf}

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err := jpeg.Encode(writer, img, &jpeg.Options{Quality: quality})
		return writer.data, "image/jpeg", err
	case "png":
		err := png.Encode(writer, img)
		return writer.data, "image/png", err
	case "webp":
		// WebP 编码，质量范围是 0-100
		err := webp.Encode(writer, img, &webp.Options{Quality: float32(quality)})
		return writer.data, "image/webp", err
	case "avif":
		// AVIF 编码，质量范围是 0-100
		err := avif.Encode(writer, img, avif.Options{Quality: quality})
		return writer.data, "image/avif", err
	default:
		// 默认使用JPEG
		err := jpeg.Encode(writer, img, &jpeg.Options{Quality: quality})
		return writer.data, "image/jpeg", err
	}
}

// bytesWriter 实现io.Writer接口的字节写入器
type bytesWriter struct {
	data []byte
}

func (w *bytesWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// init 注册AVIF解码器
func init() {
	// 注册AVIF格式解码器
	image.RegisterFormat("avif", "????ftypavif", avif.Decode, avif.DecodeConfig)
}
