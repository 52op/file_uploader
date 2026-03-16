# File Uploader

一个基于Go语言开发的高性能文件上传服务，支持多种存储后端、智能图片处理、实时访问统计和多层安全认证。

## 🚀 主要特性

### 🎯 核心功能

- **多存储支持**：本地存储、Amazon S3兼容存储，支持多实例配置
- **智能图片处理**：条件化缩略图生成，支持JPG/PNG/GIF/WebP/AVIF格式
- **安全认证**：HMAC-SHA256签名验证，防止未授权访问
- **访问控制**：Referer防盗链保护，支持域名白名单和空Referer
- **批量操作**：支持批量上传、删除、查询文件信息和文件夹管理
- **实时统计**：完整的文件访问统计、性能监控、存储使用情况
- **热配置重载**：支持配置文件热重载，无需重启服务

### 🔧 高级特性

- **HTTPS支持**：内置HTTPS服务器，支持ACME自动证书申请和续期
- **DNS自动管理**：支持阿里云DNS自动记录管理和外网IP检测
- **Prometheus监控**：完整的指标收集和监控支持
- **文件夹管理**：支持文件夹创建、列表、删除操作
- **跨平台部署**：支持Windows/Linux系统服务安装
- **防盗链保护**：支持自定义防盗链图片和中文显示

## 📦 快速开始

### 安装和运行

1. **下载并运行**
   
   ```bash
   # 下载项目
   git clone https://github.com/52op/file_uploader.git
   cd file_uploader
   
   # 编译
   go build -o file_uploader .
   
   # 运行（首次运行会自动生成配置文件）
   ./file_uploader
   ```

2. **访问服务**
   
   - 统计页面：http://localhost:8080
   - 健康检查：http://localhost:8080/health
   - Prometheus指标：http://localhost:8080/metrics

### 基本配置

首次运行会在 `config/config.yaml` 生成默认配置文件，主要配置项：

```yaml
server:
  port: "8080"
  host: 0.0.0.0

storage:
  type: local
  local:
    upload_dir: ./uploads
    base_url: http://localhost:8080/uploads

security:
  secret_key: your-secret-key-change-this-in-production
  signature_expiry: 3600

thumbnail:
  enabled: true
  width: 200
  height: 200
  min_width: 400
  min_height: 400
  min_size_kb: 50
```

## 🔧 配置说明

### 存储配置

支持多种存储后端，可同时配置多个存储实例：

```yaml
storage:
  type: local  # 默认存储类型
  enabled_storages: ["protected_images", "public_files"]

  storages:
    protected_images:
      type: local
      upload_dir: ./protected
      base_url: http://localhost:8080/protected
      require_auth: true
      allow_referer:
        - "http://localhost:3000"
        - "https://your-domain.com"
        - "blank"  # 允许空referer

    public_files:
      type: local
      upload_dir: ./public
      base_url: http://localhost:8080/public
      require_auth: false
```

### 缩略图配置

智能缩略图生成，支持条件控制：

```yaml
thumbnail:
  enabled: true           # 启用/禁用缩略图
  width: 200             # 缩略图宽度
  height: 200            # 缩略图高度
  quality: 85            # JPEG质量 (1-100)
  min_width: 400         # 原图最小宽度要求
  min_height: 400        # 原图最小高度要求
  min_size_kb: 50        # 原图最小文件大小要求（KB）
```

**缩略图智能逻辑**：

- 满足条件：生成实际缩略图，返回缩略图URL
- 不满足条件：返回原图URL作为thumbnail_url
- 功能禁用：返回原图URL作为thumbnail_url
- 简化调用端逻辑，无需额外判断

### 文件类型配置

灵活的文件类型控制，支持通配符模式：

```yaml
upload:
  max_filename_length: 100
  max_file_size: 104857600    # 最大文件大小（字节）
  allowed_extensions:         # 支持通配符的文件类型配置
    # 通配符示例
    - "*"                     # 允许所有文件类型
    - "image/*"               # 允许所有图片类型
    - "video/*"               # 允许所有视频类型
    - "application/pdf"       # 允许特定MIME类型

    # 具体扩展名
    - ".jpg"
    - ".png"
    - ".avif"                 # 支持AVIF格式
    - ".pdf"
```

**支持的通配符模式**：

- `*` - 允许所有文件类型
- `image/*` - 允许所有图片类型（基于MIME类型）
- `video/*` - 允许所有视频类型
- `application/pdf` - 允许特定MIME类型
- `.jpg` - 允许特定扩展名
- `*.jpg` - 扩展名通配符（等同于 `.jpg`）

### 图片优化配置

支持动态图片优化和格式转换：

```yaml
image_optimize:
  enabled: true
  max_width: 2048
  max_height: 2048
  default_quality: 85
  allowed_formats: ["jpeg", "png", "webp", "avif"]  # 支持AVIF输出
```

**图片优化功能**：

- 动态尺寸调整：`?w=800&h=600`
- 质量控制：`?q=80`
- 格式转换：`?f=avif` （支持转换为AVIF格式）
- 组合使用：`?w=800&h=600&q=80&f=avif`

### 安全配置

多层安全保护机制：

```yaml
security:
  secret_key: your-secret-key
  signature_expiry: 3600
  default_static_file_auth: false

# 存储级别的安全配置
storages:
  protected_storage:
    require_auth: true
    allow_referer:
      - "https://your-domain.com"
      - "localhost"
      - "blank"  # 允许空referer
```

## 📚 API文档

### 文件上传

```bash
# 基本上传
curl -X POST "http://localhost:8080/api/v1/upload" \
  -H "Authorization: Bearer your-signature" \
  -F "file=@example.jpg"

# 指定存储和路径
curl -X POST "http://localhost:8080/api/v1/upload" \
  -H "Authorization: Bearer your-signature" \
  -F "file=@example.jpg" \
  -F "storage=protected_images" \
  -F "path=/images/2024/example.jpg"
```

**响应示例**：

```json
{
  "success": true,
  "url": "http://localhost:8080/uploads/example.jpg",
  "thumbnail_url": "http://localhost:8080/uploads/example_Thumbnail.jpg",
  "filename": "example.jpg",
  "size": 102400,
  "upload_time": 1704067200
}
```

### 批量操作

```bash
# 批量上传
curl -X POST "http://localhost:8080/api/v1/batch/upload" \
  -H "Authorization: Bearer your-signature" \
  -F "files=@file1.jpg" \
  -F "files=@file2.jpg" \
  -F "storage=protected_images"

# 批量删除
curl -X POST "http://localhost:8080/api/v1/batch/delete" \
  -H "Authorization: Bearer your-signature" \
  -H "Content-Type: application/json" \
  -d '{"files": ["file1.jpg", "file2.jpg"]}'
```

### 文件访问

```bash
# 公开文件访问
http://localhost:8080/uploads/example.jpg

# 需要签名的文件访问
http://localhost:8080/protected/example.jpg?signature=xxx&timestamp=xxx
```

## 🔐 签名认证

使用HMAC-SHA256算法生成访问签名：

```go
// 生成上传签名
func generateUploadSignature(secretKey, method, path string, timestamp int64) string {
    message := fmt.Sprintf("%s|%s|%d", method, path, timestamp)
    h := hmac.New(sha256.New, []byte(secretKey))
    h.Write([]byte(message))
    return hex.EncodeToString(h.Sum(nil))
}

// 生成静态文件访问签名
func generateStaticSignature(secretKey, filePath string, timestamp int64) string {
    message := fmt.Sprintf("%s|%d", filePath, timestamp)
    h := hmac.New(sha256.New, []byte(secretKey))
    h.Write([]byte(message))
    return hex.EncodeToString(h.Sum(nil))
}
```

## 📊 监控和统计

### 实时统计页面

访问 http://localhost:8080 查看：

- 文件上传/删除/访问统计
- 存储使用情况分布
- 性能指标图表
- 缩略图生成统计
- 错误统计信息

### Prometheus监控

内置Prometheus指标收集：

```bash
# 访问指标端点
curl http://localhost:8080/metrics
```

主要指标：

- `file_uploader_uploads_total` - 上传总数
- `file_uploader_upload_duration_seconds` - 上传耗时
- `file_uploader_storage_usage_bytes` - 存储使用量
- `file_uploader_active_connections` - 活跃连接数
- `file_uploader_thumbnail_generated_total` - 缩略图生成数量

## 🚀 部署指南

### Docker部署

```bash
# 构建镜像
docker build -t file_uploader .

# 运行容器
docker run -d \
  -p 8080:8080 \
  -v ./config:/app/config \
  -v ./uploads:/app/uploads \
  --name file_uploader \
  file_uploader
```

### 系统服务安装

```bash
# Linux系统服务
sudo ./file_uploader install-service

# Windows服务
./file_uploader.exe install-service
```

### HTTPS和证书

```yaml
server:
  https:
    enabled: true
    port: "8443"
    acme:
      enabled: true
      email: "your-email@example.com"
      domains: ["your-domain.com"]
      dns:
        provider: "alidns"
        auto_dns_record: true
        external_ip_apis:
          - "https://api.ipify.org"
          - "https://ipv4.icanhazip.com"
```

## 🛠️ 开发和集成

### 客户端SDK示例

项目提供多种语言的客户端示例：

- Go客户端：`examples/client_signature.go`
- Python客户端：`examples/simple_test.py`
- Bun/Next.js集成：`examples/bun-backend/`

### 自定义存储后端

实现 `Storage` 接口即可添加新的存储后端：

```go
type Storage interface {
    Upload(filename string, file multipart.File) (string, error)
    UploadReader(filename string, reader io.Reader) (string, error)
    Delete(filename string) error
    Exists(filename string) (bool, error)
    GetURL(filename string) string
    GetFileSize(filename string) (int64, error)
}
```

## 📖 文档目录

详细文档请查看 `docs/` 目录：

- [Bun/Next.js调用流程](docs/bun-next_js调用流程.md) - 前端集成指南
- [API调用说明](docs/调用说明.md) - 完整API文档和签名生成
- [Referer防盗链配置](docs/Referer防盗链配置说明.md) - 防盗链配置详解
- [自动DNS配置](docs/自动dns注意.md) - DNS自动管理说明

## 🎯 新功能亮点

### ✅ 最新更新

- **AVIF格式支持**：完整支持AVIF图片格式的上传、优化和缩略图生成
- **通配符文件类型配置**：支持 `*`、`image/*`、`video/*` 等通配符模式
- **灵活文件类型控制**：可在配置文件中动态调整允许的文件类型
- **图片格式转换**：支持动态转换为AVIF等现代图片格式
- **静态文件访问统计**：所有文件访问都会正确记录到统计数据
- **缩略图配置参数化**：所有缩略图参数可在配置文件中调整
- **智能缩略图返回**：API始终返回thumbnail_url，简化调用端逻辑

### 🔧 配置灵活性

- 缩略图生成条件完全可配置
- 支持禁用缩略图功能
- 多存储实例独立安全配置
- 热配置重载支持

## 🤝 贡献

欢迎提交Issue和Pull Request！

## 📄 许可证

MIT License

## 🔗 相关链接

- GitHub: https://github.com/52op/file_uploader
- 博客: https://blog.sztcrs.com
- 演示站点: https://wuhu-cdn.hxljzz.com:8443/
