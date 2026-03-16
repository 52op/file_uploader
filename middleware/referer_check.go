package middleware

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"file_uploader/config"
	"file_uploader/storage"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// RefererCheckMiddleware Referer检查中间件
type RefererCheckMiddleware struct {
	storageManager *storage.StorageManager
	config         *config.Config
}

// NewRefererCheckMiddleware 创建Referer检查中间件
func NewRefererCheckMiddleware(storageManager *storage.StorageManager, cfg *config.Config) *RefererCheckMiddleware {
	return &RefererCheckMiddleware{
		storageManager: storageManager,
		config:         cfg,
	}
}

// CheckReferer 检查Referer的中间件函数
func (m *RefererCheckMiddleware) CheckReferer() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从URL路径中提取存储类型
		storageType := m.extractStorageTypeFromPath(c.Request.URL.Path)

		// 获取存储安全配置
		securityConfig := m.storageManager.GetStorageSecurityConfig(storageType)

		// 如果没有配置allow_referer，则跳过检查
		if len(securityConfig.AllowReferer) == 0 {
			c.Next()
			return
		}

		// 获取请求的Referer
		referer := c.GetHeader("Referer")

		// 检查Referer是否被允许
		if m.isRefererAllowed(referer, securityConfig.AllowReferer) {
			c.Next()
			return
		}

		// Referer检查失败，返回防盗链页面或图片
		m.handleRefererDenied(c, storageType)
		c.Abort() // 阻止后续处理器执行
	}
}

// extractStorageTypeFromPath 从URL路径中提取存储类型
func (m *RefererCheckMiddleware) extractStorageTypeFromPath(path string) string {
	// 移除开头的斜杠
	path = strings.TrimPrefix(path, "/")

	// 分割路径
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "default"
	}

	pathPrefix := "/" + parts[0]

	// 首先检查是否是已知的存储名称（直接匹配存储名称）
	availableStorages := m.storageManager.GetAvailableStorages()
	for _, storageType := range availableStorages {
		if parts[0] == storageType {
			return storageType
		}
	}

	// 然后通过路径前缀匹配存储类型
	// 获取所有存储的路径映射
	storagePathMap := m.storageManager.GetStoragePathMapping()
	for storageName, storagePath := range storagePathMap {
		if storagePath == pathPrefix {
			return storageName
		}
	}

	// 如果都没匹配到，返回默认存储
	return "default"
}

// isRefererAllowed 检查Referer是否被允许
func (m *RefererCheckMiddleware) isRefererAllowed(referer string, allowedReferers []string) bool {
	// 如果没有Referer（直接访问）
	if referer == "" {
		// 检查是否允许空referer
		for _, allowed := range allowedReferers {
			if allowed == "blank" || allowed == "" {
				return true
			}
		}
		return false
	}
	
	// 解析Referer URL
	refererURL, err := url.Parse(referer)
	if err != nil {
		return false
	}
	
	// 检查每个允许的referer
	for _, allowed := range allowedReferers {
		if allowed == "blank" || allowed == "" {
			continue // 跳过空referer配置
		}
		
		// 支持多种匹配模式
		if m.matchReferer(refererURL, allowed) {
			return true
		}
	}
	
	return false
}

// matchReferer 匹配Referer规则
func (m *RefererCheckMiddleware) matchReferer(refererURL *url.URL, allowedPattern string) bool {
	// 1. 完整URL匹配
	if refererURL.String() == allowedPattern {
		return true
	}
	
	// 2. 域名匹配（包含协议）
	if strings.HasPrefix(allowedPattern, "http://") || strings.HasPrefix(allowedPattern, "https://") {
		allowedURL, err := url.Parse(allowedPattern)
		if err == nil {
			// 精确域名匹配
			if refererURL.Host == allowedURL.Host {
				return true
			}
			// 子域名匹配（如果允许的是 .example.com）
			if strings.HasPrefix(allowedURL.Host, ".") {
				domain := strings.TrimPrefix(allowedURL.Host, ".")
				if refererURL.Host == domain || strings.HasSuffix(refererURL.Host, "."+domain) {
					return true
				}
			}
		}
		return false
	}
	
	// 3. 域名匹配（不包含协议）
	if strings.Contains(allowedPattern, ".") {
		// 精确域名匹配
		if refererURL.Host == allowedPattern {
			return true
		}
		// 子域名匹配
		if strings.HasPrefix(allowedPattern, ".") {
			domain := strings.TrimPrefix(allowedPattern, ".")
			if refererURL.Host == domain || strings.HasSuffix(refererURL.Host, "."+domain) {
				return true
			}
		}
		return false
	}
	
	// 4. 简单字符串包含匹配（用于localhost等）
	return strings.Contains(refererURL.Host, allowedPattern)
}

// isImageFile 检查是否是图片文件
func (m *RefererCheckMiddleware) isImageFile(ext string) bool {
	// 使用配置的图片文件类型，如果配置为空则使用默认值
	imageTypes := m.config.Security.AllowedFileTypes.Images
	if len(imageTypes) == 0 {
		// 默认图片类型
		imageTypes = []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".bmp", ".svg", ".ico"}
	}

	for _, imageType := range imageTypes {
		if ext == imageType {
			return true
		}
	}
	return false
}

// isVideoFile 检查是否是视频文件
func (m *RefererCheckMiddleware) isVideoFile(ext string) bool {
	// 使用配置的视频文件类型，如果配置为空则使用默认值
	videoTypes := m.config.Security.AllowedFileTypes.Videos
	if len(videoTypes) == 0 {
		// 默认视频类型
		videoTypes = []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".mkv", ".m4v"}
	}

	for _, videoType := range videoTypes {
		if ext == videoType {
			return true
		}
	}
	return false
}

// handleRefererDenied 处理Referer检查失败的情况
func (m *RefererCheckMiddleware) handleRefererDenied(c *gin.Context, storageType string) {
	// 记录防盗链日志
	clientIP := c.ClientIP()
	referer := c.GetHeader("Referer")
	userAgent := c.GetHeader("User-Agent")
	requestPath := c.Request.URL.Path
	
	fmt.Printf("[ANTI-HOTLINK] 阻止盗链访问 - IP: %s, Referer: %s, Path: %s, UserAgent: %s, Storage: %s\n",
		clientIP, referer, requestPath, userAgent, storageType)
	
	// 根据请求的文件类型返回不同的防盗链内容
	ext := strings.ToLower(filepath.Ext(requestPath))

	// 检查是否是图片文件
	if m.isImageFile(ext) {
		// 返回防盗链图片
		m.serveAntiHotlinkImage(c)
	} else if m.isVideoFile(ext) {
		// 返回防盗链视频或错误信息
		m.serveAntiHotlinkVideo(c)
	} else {
		// 返回防盗链HTML页面
		m.serveAntiHotlinkPage(c)
	}
}

// serveAntiHotlinkImage 返回防盗链图片
func (m *RefererCheckMiddleware) serveAntiHotlinkImage(c *gin.Context) {
	// 检查是否存在自定义防盗链图片（从配置读取路径）
	antiHotlinkImagePath := m.config.Upload.AntiHotlinkImage
	if antiHotlinkImagePath == "" {
		antiHotlinkImagePath = "./static/anti-hotlink.png" // 默认路径
	}

	// 检查文件是否存在
	if _, err := os.Stat(antiHotlinkImagePath); err == nil {
		c.File(antiHotlinkImagePath)
		return
	}
	
	// 如果防盗链图片不存在，返回默认的防盗链图片
	m.serveDefaultAntiHotlinkImage(c)
}

// serveAntiHotlinkVideo 返回防盗链视频处理
func (m *RefererCheckMiddleware) serveAntiHotlinkVideo(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.JSON(http.StatusForbidden, gin.H{
		"error":   "访问被拒绝",
		"message": "该视频文件不允许外部引用",
		"code":    "HOTLINK_DENIED",
	})
}

// serveAntiHotlinkPage 返回防盗链HTML页面
func (m *RefererCheckMiddleware) serveAntiHotlinkPage(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>访问被拒绝</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; background-color: #f5f5f5; }
        .container { max-width: 600px; margin: 0 auto; background: white; padding: 40px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .icon { font-size: 64px; color: #e74c3c; margin-bottom: 20px; }
        h1 { color: #2c3e50; margin-bottom: 20px; }
        p { color: #7f8c8d; line-height: 1.6; margin-bottom: 15px; }
        .code { background: #ecf0f1; padding: 10px; border-radius: 5px; font-family: monospace; color: #2c3e50; }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">🚫</div>
        <h1>访问被拒绝</h1>
        <p>该资源不允许外部网站直接引用。</p>
        <p>如果您是网站管理员，请检查配置文件。</p>
        <div class="code">错误代码: HOTLINK_DENIED</div>
    </div>
</body>
</html>`
	
	c.Data(http.StatusForbidden, "text/html; charset=utf-8", []byte(html))
}

// serveDefaultAntiHotlinkImage 返回默认的防盗链图片
func (m *RefererCheckMiddleware) serveDefaultAntiHotlinkImage(c *gin.Context) {
	// 生成一个包含提示文字的PNG图片
	img := m.generateDefaultAntiHotlinkImage()

	// 将图片编码为PNG格式
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		// 如果生成失败，返回简单的透明图片
		m.serveSimpleTransparentImage(c)
		return
	}

	c.Header("Content-Type", "image/png")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Data(http.StatusForbidden, "image/png", buf.Bytes())
}

// generateDefaultAntiHotlinkImage 生成默认的防盗链图片
func (m *RefererCheckMiddleware) generateDefaultAntiHotlinkImage() image.Image {
	// 创建一个400x300的图片
	width, height := 400, 300
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 设置背景色为浅灰色
	bgColor := color.RGBA{240, 240, 240, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// 绘制一个简单的禁止符号（圆圈+斜线）
	m.drawProhibitSymbol(img, width/2, 80, 30)

	// 设置文字颜色为深灰色
	textColor := color.RGBA{80, 80, 80, 255}

	// 绘制文字
	face := basicfont.Face7x13
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(textColor),
		Face: face,
	}

	// 绘制多行文字（使用英文避免中文乱码）
	lines := []string{
		"Access Denied",
		"",
		"This resource does not allow hotlinking.",
		"",
		"If you are the administrator,",
		"please check your configuration file.",
		"",
		"Error Code: HOTLINK_DENIED",
		"",
		"Tip: Replace this default image",
	}

	lineHeight := 20
	startY := 50

	for i, line := range lines {
		// 计算文字居中位置
		textWidth := font.MeasureString(face, line)
		x := (width - textWidth.Round()) / 2
		y := startY + i*lineHeight

		drawer.Dot = fixed.Point26_6{
			X: fixed.I(x),
			Y: fixed.I(y),
		}
		drawer.DrawString(line)
	}

	return img
}

// drawProhibitSymbol 绘制禁止符号（圆圈+斜线）
func (m *RefererCheckMiddleware) drawProhibitSymbol(img *image.RGBA, centerX, centerY, radius int) {
	red := color.RGBA{200, 50, 50, 255}

	// 绘制圆圈
	for y := centerY - radius; y <= centerY + radius; y++ {
		for x := centerX - radius; x <= centerX + radius; x++ {
			dx := x - centerX
			dy := y - centerY
			distance := dx*dx + dy*dy

			// 圆圈边框（粗线）
			if distance >= (radius-3)*(radius-3) && distance <= radius*radius {
				if x >= 0 && x < img.Bounds().Max.X && y >= 0 && y < img.Bounds().Max.Y {
					img.Set(x, y, red)
				}
			}
		}
	}

	// 绘制斜线（从左上到右下）
	for i := -radius + 5; i <= radius - 5; i++ {
		for thickness := -2; thickness <= 2; thickness++ {
			x := centerX + i
			y := centerY + i + thickness
			if x >= 0 && x < img.Bounds().Max.X && y >= 0 && y < img.Bounds().Max.Y {
				img.Set(x, y, red)
			}
		}
	}
}

// serveSimpleTransparentImage 返回简单的透明图片（备用方案）
func (m *RefererCheckMiddleware) serveSimpleTransparentImage(c *gin.Context) {
	c.Header("Content-Type", "image/png")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")

	// 1x1透明PNG图片的数据
	transparentPNG := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	c.Data(http.StatusForbidden, "image/png", transparentPNG)
}
