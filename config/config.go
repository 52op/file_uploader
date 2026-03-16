package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"gopkg.in/yaml.v2"
)

// Config 应用配置结构
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Storage       StorageConfig       `yaml:"storage"`
	Security      SecurityConfig      `yaml:"security"`
	Upload        UploadConfig        `yaml:"upload"`
	Network       NetworkConfig       `yaml:"network"`
	Thumbnail     ThumbnailConfig     `yaml:"thumbnail"`
	ImageOptimize ImageOptimizeConfig `yaml:"image_optimize"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port  string      `yaml:"port"`
	Host  string      `yaml:"host"`
	HTTPS HTTPSConfig `yaml:"https"`
}

// HTTPSConfig HTTPS配置
type HTTPSConfig struct {
	Enabled  bool       `yaml:"enabled"`
	CertFile string     `yaml:"cert_file"`
	KeyFile  string     `yaml:"key_file"`
	Port     string     `yaml:"port"`
	ACME     ACMEConfig `yaml:"acme"`
}

// ACMEConfig ACME自动证书配置
type ACMEConfig struct {
	Enabled     bool              `yaml:"enabled"`      // 是否启用ACME自动证书
	Email       string            `yaml:"email"`        // Let's Encrypt注册邮箱
	Domains     []string          `yaml:"domains"`      // 需要申请证书的域名列表
	Server      string            `yaml:"server"`       // ACME服务器地址，默认Let's Encrypt
	CertDir     string            `yaml:"cert_dir"`     // 证书存储目录
	RenewBefore int               `yaml:"renew_before"` // 证书到期前多少天开始续期，默认30天
	DNS         DNSConfig         `yaml:"dns"`          // DNS验证配置
	KeyType     string            `yaml:"key_type"`     // 密钥类型：RSA2048, RSA4096, EC256, EC384
}

// DNSConfig DNS验证配置
type DNSConfig struct {
	Provider         string                 `yaml:"provider"`           // DNS提供商：alidns, tencentcloud, cloudflare
	Config           map[string]interface{} `yaml:"config"`             // DNS提供商特定配置
	AutoDNSRecord    bool                   `yaml:"auto_dns_record"`    // 是否启用自动DNS记录管理
	ExternalIPAPIs   []string               `yaml:"external_ip_apis"`   // 外网IP检测API列表
	ExternalIP       string                 `yaml:"external_ip"`        // 手动指定外网IP（可选）
	DNSCheckInterval int                    `yaml:"dns_check_interval"` // DNS检查间隔（分钟），0表示只在启动时检查
}

// StorageConfig 存储配置
type StorageConfig struct {
	Type            string                 `yaml:"type"`             // 默认存储类型
	EnabledStorages []string               `yaml:"enabled_storages"` // 启用的存储列表
	Local           LocalConfig            `yaml:"local"`
	S3              S3Config               `yaml:"s3"`
	Storages        map[string]interface{} `yaml:"storages"` // 多存储配置
}

// LocalConfig 本地存储配置
type LocalConfig struct {
	UploadDir string `yaml:"upload_dir"` // 上传文件存储目录
	BaseURL   string `yaml:"base_url"`   // 文件访问基础URL
}

// S3Config S3存储配置
type S3Config struct {
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	BaseURL         string `yaml:"base_url"` // 文件访问基础URL，如 https://domain.com
	Endpoint        string `yaml:"endpoint"` // 自定义S3端点（可选）
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	SecretKey               string                 `yaml:"secret_key"`                 // HMAC签名密钥
	SignatureExpiry         int64                  `yaml:"signature_expiry"`           // 签名有效期（秒）
	DefaultStaticFileAuth   bool                   `yaml:"default_static_file_auth"`   // 静态文件访问的全局默认设置
	AllowedFileTypes        AllowedFileTypesConfig `yaml:"allowed_file_types"`         // 允许的文件类型配置
}

// ThumbnailConfig 缩略图配置
type ThumbnailConfig struct {
	Enabled     bool  `yaml:"enabled"`      // 是否启用缩略图生成
	Width       int   `yaml:"width"`        // 缩略图宽度
	Height      int   `yaml:"height"`       // 缩略图高度
	Quality     int   `yaml:"quality"`      // JPEG质量 (1-100)
	MinWidth    int   `yaml:"min_width"`    // 原图最小宽度（小于此值不生成缩略图）
	MinHeight   int   `yaml:"min_height"`   // 原图最小高度（小于此值不生成缩略图）
	MinSizeKB   int   `yaml:"min_size_kb"`  // 原图最小文件大小（KB，小于此值不生成缩略图）
}

// AllowedFileTypesConfig 允许的文件类型配置
type AllowedFileTypesConfig struct {
	Images []string `yaml:"images"` // 图片文件扩展名列表
	Videos []string `yaml:"videos"` // 视频文件扩展名列表
}

// UploadConfig 上传配置
type UploadConfig struct {
	MaxFilenameLength int      `yaml:"max_filename_length"` // 最大文件名长度
	AntiHotlinkImage  string   `yaml:"anti_hotlink_image"`  // 防盗链图片路径
	AllowedExtensions []string `yaml:"allowed_extensions"`  // 允许上传的文件扩展名列表
	MaxFileSize       int64    `yaml:"max_file_size"`       // 最大文件大小（字节）
}

// NetworkConfig 网络配置
type NetworkConfig struct {
	RequestTimeout       string `yaml:"request_timeout"`        // HTTP请求超时时间
	LogRotationInterval  string `yaml:"log_rotation_interval"`  // 日志轮转检查间隔
}

// ImageOptimizeConfig 图片优化配置
type ImageOptimizeConfig struct {
	Enabled        bool     `yaml:"enabled"`         // 是否启用图片优化
	MaxWidth       int      `yaml:"max_width"`       // 最大宽度限制
	MaxHeight      int      `yaml:"max_height"`      // 最大高度限制
	DefaultQuality int      `yaml:"default_quality"` // 默认质量 (1-100)
	AllowedFormats []string `yaml:"allowed_formats"` // 允许的输出格式：jpeg, png, webp, avif
}



var globalConfig *Config

// GetStorageAuthRequirement 获取指定存储的认证要求
// 如果存储配置中没有设置 require_auth，则使用全局默认值
func (c *Config) GetStorageAuthRequirement(storageName string) bool {
	// 如果是默认存储，使用全局默认设置
	if storageName == "" || storageName == "uploads" {
		return c.Security.DefaultStaticFileAuth
	}

	// 检查多存储配置
	if c.Storage.Storages != nil {
		if storageConfig, exists := c.Storage.Storages[storageName]; exists {
			if configMap, ok := storageConfig.(map[interface{}]interface{}); ok {
				// 查找 require_auth 配置
				if requireAuthInterface, exists := configMap["require_auth"]; exists {
					if requireAuth, ok := requireAuthInterface.(bool); ok {
						return requireAuth
					}
				}
			}
		}
	}

	// 如果没有找到特定配置，使用全局默认值
	return c.Security.DefaultStaticFileAuth
}

// LoadConfig 从文件加载配置
func LoadConfig(configPath string) (*Config, error) {
	// 如果配置文件不存在，创建默认配置文件
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := createDefaultConfig(configPath); err != nil {
			return nil, fmt.Errorf("创建默认配置文件失败: %v", err)
		}
	}

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %v", err)
	}

	globalConfig = &config
	return &config, nil
}

// GetConfig 获取全局配置（线程安全）
func GetConfig() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// validateConfig 验证配置有效性
func validateConfig(config *Config) error {
	if config.Server.Port == "" {
		return fmt.Errorf("服务器端口不能为空")
	}

	if config.Security.SecretKey == "" {
		return fmt.Errorf("安全密钥不能为空")
	}

	if config.Security.SignatureExpiry <= 0 {
		config.Security.SignatureExpiry = 3600 // 默认1小时
	}

	switch config.Storage.Type {
	case "local":
		if config.Storage.Local.UploadDir == "" {
			return fmt.Errorf("本地存储目录不能为空")
		}
		if config.Storage.Local.BaseURL == "" {
			return fmt.Errorf("本地存储基础URL不能为空")
		}
	case "s3":
		if config.Storage.S3.Region == "" {
			return fmt.Errorf("S3区域不能为空")
		}
		if config.Storage.S3.Bucket == "" {
			return fmt.Errorf("S3存储桶不能为空")
		}
		if config.Storage.S3.AccessKeyID == "" {
			return fmt.Errorf("S3访问密钥ID不能为空")
		}
		if config.Storage.S3.SecretAccessKey == "" {
			return fmt.Errorf("S3访问密钥不能为空")
		}
		if config.Storage.S3.BaseURL == "" {
			return fmt.Errorf("S3基础URL不能为空")
		}
	default:
		return fmt.Errorf("不支持的存储类型: %s", config.Storage.Type)
	}

	return nil
}

// createDefaultConfig 创建默认配置文件
func createDefaultConfig(configPath string) error {
	// 手动构建YAML内容，包含详细注释
	yamlContent := `server:
  port: "8080"
  host: 0.0.0.0
  https:
    enabled: false
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    port: "8443"
    # ACME自动证书配置
    acme:
      enabled: false                    # 是否启用ACME自动证书
      email: "your-email@example.com"   # Let's Encrypt注册邮箱
      domains:                          # 需要申请证书的域名列表
        - "example.com"
      server: ""                        # ACME服务器地址，空值使用Let's Encrypt生产环境
      cert_dir: "./certs"               # 证书存储目录
      renew_before: 30                  # 证书到期前多少天开始续期
      key_type: "RSA2048"               # 密钥类型：RSA2048, RSA4096, EC256, EC384
      # DNS验证配置
      dns:
        provider: "alidns"              # DNS提供商：alidns, tencentcloud, cloudflare
        auto_dns_record: false          # 启用/禁用自动DNS记录管理（避免权限问题）
        external_ip_apis:               # 外网IP检测API列表（按顺序尝试）
          - "https://api.ipify.org"
          - "https://ipv4.icanhazip.com"
          - "https://checkip.amazonaws.com"
          - "https://ipinfo.io/ip"
          - "https://api.myip.com"
        # external_ip: ""               # 手动指定外网IP（可选，留空则自动检测）
        dns_check_interval: 0           # DNS检查间隔（分钟），0表示只在启动时检查
        config:
          # 阿里云DNS配置示例
          access_key_id: "your-access-key-id"
          access_key_secret: "your-access-key-secret"
          region: "cn-hangzhou"

storage: # 注意：只有配置在多存储storages下面的存储支持require_auth和allow_referer安全特性
  type: local  # 默认存储类型：local 或 s3
  # 启用的存储列表（如果为空则启用所有配置的存储）
  enabled_storages: []  # 示例: ["local_backup", "public_storage"]

  # 默认本地存储配置（仅支持基本功能，不支持安全特性）
  local:
    upload_dir: ./uploads
    base_url: http://localhost:8080/uploads

  # 默认S3存储配置（仅支持基本功能，不支持安全特性）
  s3:
    region: us-east-1
    bucket: your-bucket-name
    access_key_id: your-access-key-id
    secret_access_key: your-secret-access-key
    base_url: https://your-domain.com
    endpoint: ""

  # 多存储配置示例（支持完整的安全特性）
  storages:
    # S3存储示例
    s3_backup:
      type: s3
      region: us-west-2
      bucket: backup-bucket
      access_key_id: backup-access-key
      secret_access_key: backup-secret-key
      base_url: https://backup.example.com
      endpoint: ""
      # S3存储不需要allow_referer配置（因为不提供静态文件服务）

    # 本地存储示例（带防盗链保护）
    protected_images:
      type: local
      upload_dir: ./protected
      base_url: http://localhost:8080/protected
      require_auth: true          # 需要签名验证
      allow_referer:              # Referer白名单，如果配置则只允许指定referer访问
        - "http://localhost:3000"
        - "https://your-domain.com"
        - "blank"                 # 允许空referer（直接访问）
        - "localhost"             # 开发环境

    # 公开存储示例（无安全限制）
    public_files:
      type: local
      upload_dir: ./public
      base_url: http://localhost:8080/public
      require_auth: false         # 公开访问，不需要签名
      # 不配置allow_referer，表示不进行Referer检查

security:
  secret_key: your-secret-key-change-this-in-production
  signature_expiry: 3600  # 签名有效期（秒），默认1小时
  default_static_file_auth: false  # 静态文件访问的全局默认设置，false=公开访问，true=需要签名
  # 允许的文件类型配置（用于防盗链响应）
  allowed_file_types:
    images: [".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".bmp", ".svg", ".ico"]
    videos: [".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".mkv", ".m4v"]

# 上传配置
upload:
  max_filename_length: 100                    # 最大文件名长度
  anti_hotlink_image: "./static/anti-hotlink.png"  # 防盗链图片路径
  max_file_size: 104857600                    # 最大文件大小（字节），默认100MB
  allowed_extensions:                         # 允许上传的文件扩展名列表（支持通配符）
    # 通配符示例：
    # - "*"                    # 允许所有文件类型
    # - "image/*"              # 允许所有图片类型（基于MIME类型）
    # - "video/*"              # 允许所有视频类型（基于MIME类型）
    # - "application/pdf"      # 允许特定MIME类型
    # - "*.jpg"                # 允许特定扩展名（等同于 ".jpg"）

    # 图片文件
    - ".jpg"
    - ".jpeg"
    - ".png"
    - ".gif"
    - ".webp"
    - ".avif"
    - ".bmp"
    - ".svg"
    - ".ico"
    # 文档文件
    - ".pdf"
    - ".txt"
    - ".doc"
    - ".docx"
    - ".xls"
    - ".xlsx"
    - ".ppt"
    - ".pptx"
    # 压缩文件
    - ".zip"
    - ".rar"
    - ".7z"
    - ".tar"
    - ".gz"
    # 音视频文件
    - ".mp4"
    - ".avi"
    - ".mov"
    - ".wmv"
    - ".flv"
    - ".webm"
    - ".mkv"
    - ".m4v"
    - ".mp3"
    - ".wav"
    - ".flac"
    - ".aac"

# 网络配置
network:
  request_timeout: "10s"          # HTTP请求超时时间
  log_rotation_interval: "1h"     # 日志轮转检查间隔

# 缩略图配置
thumbnail:
  enabled: true                   # 是否启用缩略图生成
  width: 200                      # 缩略图宽度（像素）
  height: 200                     # 缩略图高度（像素）
  quality: 85                     # JPEG质量 (1-100)
  min_width: 400                  # 原图最小宽度（小于此值不生成缩略图）
  min_height: 400                 # 原图最小高度（小于此值不生成缩略图）
  min_size_kb: 50                 # 原图最小文件大小（KB，小于此值不生成缩略图）

# 图片优化配置
image_optimize:
  enabled: true                   # 是否启用图片优化
  max_width: 2048                 # 最大宽度限制（像素）
  max_height: 2048                # 最大高度限制（像素）
  default_quality: 85             # 默认质量 (1-100)
  allowed_formats: ["jpeg", "png", "webp", "avif"]  # 允许的输出格式
`

	// 创建配置文件目录
	if err := os.MkdirAll("config", 0755); err != nil {
		return err
	}

	return ioutil.WriteFile(configPath, []byte(yamlContent), 0644)
}

// 全局配置管理
var (
	configMutex  sync.RWMutex
)

// SetGlobalConfig 设置全局配置
func SetGlobalConfig(config *Config) {
	configMutex.Lock()
	defer configMutex.Unlock()
	globalConfig = config
}
