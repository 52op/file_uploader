# 📚 File Uploader API 说明文档

本文档详细说明了 File Uploader 服务的所有 API 端点，包括请求方式、参数、响应格式等。

## 🔐 认证说明

大部分 API 端点需要 HMAC-SHA256 签名认证，签名参数通过查询字符串传递：

```
?expires=时间戳&signature=签名值
```

### 签名生成规则

**重要**：所有客户端必须严格遵循以下规则：

1. **路径格式**：使用**未编码的原始路径**生成签名（包含中文等特殊字符）
2. **签名格式**：HMAC(32字节) + 随机数(8字节) = 40字节，转换为80个十六进制字符
3. **签名内容**：`路径 + 过期时间戳 + 随机数(十六进制)`
4. **密钥**：配置文件中的 `security.secret_key`

### 路径处理规范

- ✅ **正确**：`/api/v1/files/hxljzz/images/微信图片_example.jpg`
- ❌ **错误**：`/api/v1/files/hxljzz/images/%E5%BE%AE%E4%BF%A1%E5%9B%BE%E7%89%87_example.jpg`

**说明**：
- 客户端生成签名时使用未编码的原始路径
- HTTP请求时由HTTP库自动处理URL编码
- 服务端会自动解码URL后进行签名验证

## 📋 API 端点总览

### 🔓 公开端点（无需签名）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |
| GET | `/` | 统计仪表板页面 |
| GET | `/api/stats` | 获取统计数据 |
| GET | `/metrics` | Prometheus 指标 |

### 🔒 需要签名认证的端点

#### 文件操作 API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/upload` | 单文件上传 |
| DELETE | `/api/v1/files/*filepath` | 删除文件 |
| GET | `/api/v1/files/*filepath` | 获取文件信息 |

#### 批量操作 API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/batch/upload` | 批量文件上传 |
| POST | `/api/v1/batch/delete` | 批量文件删除 |
| POST | `/api/v1/batch/info` | 批量文件信息查询 |

#### 文件夹操作 API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/folders` | 创建文件夹 |
| POST | `/api/v1/folders/list` | 列出文件夹内容 |
| DELETE | `/api/v1/folders/*path` | 删除文件夹 |

#### 配置管理 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/config/info` | 获取配置信息 |
| POST | `/api/v1/config/reload` | 重载配置 |

#### 证书管理 API（ACME 启用时）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/cert/info` | 获取证书信息 |
| POST | `/api/v1/cert/obtain` | 申请证书 |
| POST | `/api/v1/cert/renew` | 续期证书 |
| POST | `/api/v1/cert/ensure` | 确保证书可用 |
| GET | `/api/v1/cert/status` | 获取 ACME 状态 |

#### 静态文件服务

| 路径模式 | 说明 | 认证要求 |
|----------|------|----------|
| `/{storage_name}/*filepath` | 访问指定存储的静态文件 | 根据存储配置 |
| `/uploads/*filepath` | 访问默认存储的静态文件 | 根据存储配置 |

#### 调试端点（仅开发模式）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/debug/signature` | 查看签名信息 |
| GET | `/debug/config` | 查看配置信息 |
| GET | `/debug/acme` | 查看 ACME 状态 |

---

## 📤 详细 API 说明

### 1. 健康检查

**端点**: `GET /health`

**说明**: 检查服务器运行状态

**请求参数**: 无

**响应示例**:
```json
{
  "status": "ok",
  "timestamp": 1704067200,
  "uptime": "2h30m15s"
}
```

### 2. 统计数据

**端点**: `GET /api/stats`

**说明**: 获取服务器统计数据

**请求参数**: 无

**响应示例**:
```json
{
  "total_uploads": 1250,
  "total_deletes": 45,
  "total_accesses": 8920,
  "total_files": 1205,
  "total_size_mb": 2048.5,
  "batch_uploads": 120,
  "upload_errors": 5,
  "delete_errors": 1,
  "access_errors": 12,
  "batch_errors": 2,
  "thumbnails_generated": 890,
  "thumbnail_errors": 8,
  "avg_upload_time": "1.2s",
  "avg_file_size": "1.7MB",
  "uptime_human": "2d 5h 30m",
  "last_update": "2024-01-01 12:30:45",
  "storage_count": 3
}
```

### 3. 单文件上传

**端点**: `POST /api/v1/upload`

**认证**: 需要签名

**请求参数**:
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
- **表单数据**:
  - `file`: 上传的文件（必需）
  - `storage`: 存储类型（可选）
  - `path`: 自定义路径（可选）

**支持的文件类型**:
- **图片格式**: JPG、PNG、GIF、WebP、**AVIF**、BMP、SVG、ICO
- **文档格式**: PDF、TXT、DOC、DOCX、XLS、XLSX、PPT、PPTX
- **压缩格式**: ZIP、RAR、7Z、TAR、GZ
- **音视频格式**: MP4、AVI、MOV、WMV、FLV、WebM、MKV、MP3、WAV、FLAC、AAC
- **通配符配置**: 支持在配置文件中使用通配符模式控制允许的文件类型

**文件类型配置示例**:
```yaml
upload:
  allowed_extensions:
    - "*"                     # 允许所有文件类型
    - "image/*"               # 允许所有图片类型
    - "video/*"               # 允许所有视频类型
    - "application/pdf"       # 允许特定MIME类型
    - ".avif"                 # 允许AVIF格式
```

**请求示例**:
```bash
curl -X POST "http://localhost:8080/api/v1/upload?expires=1735977600&signature=abc123..." \
  -F "file=@example.jpg" \
  -F "storage=protected_images" \
  -F "path=/images/2024/example.jpg"
```

**响应示例**:
```json
{
  "success": true,
  "url": "http://localhost:8080/uploads/example.jpg",
  "thumbnail_url": "http://localhost:8080/uploads/example_Thumbnail.jpg",
  "filename": "example.jpg",
  "size": 102400,
  "content_type": "image/jpeg",
  "upload_time": 1704067200,
  "storage_type": "local"
}
```

### 4. 删除文件

**端点**: `DELETE /api/v1/files/*filepath`

**认证**: 需要签名

**说明**: `filepath` 参数支持包含完整路径的文件名，可以删除任意目录层级的文件。

**请求参数**:
- **路径参数**:
  - `filepath`: 要删除的文件路径（必需，支持包含目录的完整路径）
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
  - `storage`: 存储类型（可选）

**两种调用方式**:

#### 方式1：路径中包含存储类型（推荐）
```bash
# 删除 hxljzz 存储中的文件
curl -X DELETE "http://localhost:8080/api/v1/files/hxljzz/images/20250710/example.jpg?expires=1735977600&signature=abc123..."

# 删除 s3 存储中的文件
curl -X DELETE "http://localhost:8080/api/v1/files/s3/documents/2024/report.pdf?expires=1735977600&signature=abc123..."

# 删除默认存储中的文件
curl -X DELETE "http://localhost:8080/api/v1/files/images/20250710/example.jpg?expires=1735977600&signature=abc123..."
```

#### 方式2：使用 storage 查询参数
```bash
curl -X DELETE "http://localhost:8080/api/v1/files/images/20250710/example.jpg?expires=1735977600&signature=abc123...&storage=hxljzz"
```

**响应示例**:
```json
{
  "success": true,
  "message": "文件删除成功"
}
```

### 5. 获取文件信息

**端点**: `GET /api/v1/files/*filepath`

**认证**: 需要签名

**说明**: `filepath` 参数支持包含完整路径的文件名。

**请求参数**:
- **路径参数**:
  - `filepath`: 文件路径（必需，支持包含目录的完整路径）
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
  - `storage`: 存储类型（可选）

**请求示例**:
```bash
# 方式1：路径中包含存储类型
curl "http://localhost:8080/api/v1/files/hxljzz/images/20250710/example.jpg?expires=1735977600&signature=abc123..."

# 方式2：使用 storage 查询参数
curl "http://localhost:8080/api/v1/files/images/20250710/example.jpg?expires=1735977600&signature=abc123...&storage=hxljzz"
```

**响应示例**:
```json
{
  "success": true,
  "file_info": {
    "filename": "example.jpg",
    "size": 102400,
    "content_type": "image/jpeg",
    "url": "http://localhost:8080/uploads/example.jpg",
    "upload_time": 1704067200,
    "storage_type": "local"
  }
}
```

### 6. 批量文件上传

**端点**: `POST /api/v1/batch/upload`

**认证**: 需要签名

**请求参数**:
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
- **表单数据**:
  - `files`: 上传的文件列表（必需，可多个）
  - `storage`: 存储类型（可选）

**请求示例**:
```bash
curl -X POST "http://localhost:8080/api/v1/batch/upload?expires=1735977600&signature=abc123..." \
  -F "files=@file1.jpg" \
  -F "files=@file2.jpg" \
  -F "storage=protected_images"
```

**响应示例**:
```json
{
  "success": true,
  "results": [
    {
      "filename": "file1.jpg",
      "success": true,
      "url": "http://localhost:8080/uploads/file1.jpg",
      "thumbnail_url": "http://localhost:8080/uploads/file1_Thumbnail.jpg",
      "size": 102400,
      "content_type": "image/jpeg"
    },
    {
      "filename": "file2.jpg",
      "success": true,
      "url": "http://localhost:8080/uploads/file2.jpg",
      "thumbnail_url": "http://localhost:8080/uploads/file2_Thumbnail.jpg",
      "size": 204800,
      "content_type": "image/jpeg"
    }
  ],
  "total_uploaded": 2,
  "total_failed": 0,
  "storage_type": "local"
}
```

### 7. 批量文件删除

**端点**: `POST /api/v1/batch/delete`

**认证**: 需要签名

**请求参数**:
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
- **JSON 数据**:
  - `filenames`: 要删除的文件名列表（必需）
  - `storage`: 存储类型（可选）

**请求示例**:
```bash
# 方式1：文件路径包含存储类型
curl -X POST "http://localhost:8080/api/v1/batch/delete?expires=1735977600&signature=abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "filenames": ["hxljzz/images/20250710/file1.jpg", "hxljzz/images/20250710/file2.jpg"]
  }'

# 方式2：使用 storage 参数
curl -X POST "http://localhost:8080/api/v1/batch/delete?expires=1735977600&signature=abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "filenames": ["images/20250710/file1.jpg", "images/20250710/file2.jpg"],
    "storage": "hxljzz"
  }'
```

**响应示例**:
```json
{
  "success": true,
  "results": [
    {
      "filename": "file1.jpg",
      "success": true,
      "message": "删除成功"
    },
    {
      "filename": "file2.jpg",
      "success": true,
      "message": "删除成功"
    }
  ],
  "total_deleted": 2,
  "total_failed": 0
}
```

### 8. 批量文件信息查询

**端点**: `POST /api/v1/batch/info`

**认证**: 需要签名

**请求参数**:
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
- **JSON 数据**:
  - `filenames`: 要查询的文件名列表（必需）
  - `storage`: 存储类型（可选）

**响应示例**:
```json
{
  "success": true,
  "results": [
    {
      "filename": "file1.jpg",
      "success": true,
      "file_info": {
        "size": 102400,
        "content_type": "image/jpeg",
        "url": "http://localhost:8080/uploads/file1.jpg",
        "upload_time": 1704067200
      }
    }
  ]
}
```

### 9. 创建文件夹

**端点**: `POST /api/v1/folders`

**认证**: 需要签名

**请求参数**:
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
- **JSON 数据**:
  - `path`: 文件夹路径（必需）
  - `storage`: 存储类型（可选）

**响应示例**:
```json
{
  "success": true,
  "message": "文件夹创建成功",
  "path": "/images/2024",
  "storage_type": "local"
}
```

### 10. 列出文件夹内容

**端点**: `POST /api/v1/folders/list`

**认证**: 需要签名

**请求参数**:
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
- **JSON 数据**:
  - `path`: 文件夹路径（必需）
  - `storage`: 存储类型（可选）

**响应示例**:
```json
{
  "success": true,
  "path": "/images/2024",
  "files": [
    {
      "name": "photo1.jpg",
      "size": 102400,
      "is_dir": false,
      "modified_time": 1704067200
    },
    {
      "name": "subfolder",
      "size": 0,
      "is_dir": true,
      "modified_time": 1704067100
    }
  ],
  "total_files": 1,
  "total_dirs": 1
}
```

### 11. 删除文件夹

**端点**: `DELETE /api/v1/folders/*path`

**认证**: 需要签名

**请求参数**:
- **路径参数**:
  - `path`: 要删除的文件夹路径（必需，支持包含存储类型的完整路径）
- **查询参数**:
  - `expires`: 过期时间戳（必需）
  - `signature`: 签名值（必需）
  - `storage`: 存储类型（可选）

**请求示例**:
```bash
# 方式1：路径中包含存储类型
curl -X DELETE "http://localhost:8080/api/v1/folders/hxljzz/images/2024?expires=1735977600&signature=abc123..."

# 方式2：使用 storage 查询参数
curl -X DELETE "http://localhost:8080/api/v1/folders/images/2024?expires=1735977600&signature=abc123...&storage=hxljzz"
```

**响应示例**:
```json
{
  "success": true,
  "message": "文件夹删除成功",
  "path": "/images/2024",
  "storage_type": "local"
}
```

### 12. 获取配置信息

**端点**: `GET /api/v1/config/info`

**认证**: 需要签名

**响应示例**:
```json
{
  "server": {
    "port": "8080",
    "host": "0.0.0.0",
    "https_enabled": true
  },
  "storage": {
    "type": "local",
    "enabled_storages": ["local", "protected_images", "public_files"]
  },
  "security": {
    "signature_expiry": 3600
  },
  "hot_reload": {
    "enabled": true
  }
}
```

### 13. 重载配置

**端点**: `POST /api/v1/config/reload`

**认证**: 需要签名

**响应示例**:
```json
{
  "success": true,
  "message": "配置重新加载成功",
  "timestamp": 1704067200
}
```

### 14. 静态文件访问

**端点**: `GET /{storage_name}/*filepath` 或 `GET /uploads/*filepath`

**认证**: 根据存储配置决定是否需要签名

**说明**:
- 如果存储配置了 `require_auth: true`，则需要签名认证
- 如果存储配置了 `require_auth: false`，则公开访问
- 支持 Referer 防盗链检查

**请求示例**:
```bash
# 公开访问
curl "http://localhost:8080/uploads/example.jpg"

# 需要签名的访问
curl "http://localhost:8080/protected/example.jpg?expires=1735977600&signature=abc123..."
```

---

## 🔧 错误响应格式

所有 API 在出错时都会返回统一的错误格式：

```json
{
  "success": false,
  "error": "错误描述",
  "details": "详细错误信息（可选）"
}
```

**常见错误码**:
- `400 Bad Request`: 请求参数错误
- `401 Unauthorized`: 签名验证失败或已过期
- `403 Forbidden`: 权限不足
- `404 Not Found`: 文件或资源不存在
- `413 Payload Too Large`: 文件大小超过限制
- `500 Internal Server Error`: 服务器内部错误

---

## 🖼️ 图片优化功能

支持通过URL查询参数对图片进行实时优化：

### 优化参数

- `w`: 宽度（像素）
- `h`: 高度（像素）
- `q`: 质量（1-100）
- `f`: 格式（jpeg, png, webp, **avif**）

### 使用示例

```bash
# 原始图片
GET /uploads/images/example.jpg

# 调整尺寸为300x200
GET /uploads/images/example.jpg?w=300&h=200

# 调整质量为80%
GET /uploads/images/example.jpg?q=80

# 转换为WebP格式
GET /uploads/images/example.jpg?f=webp

# 转换为AVIF格式（最新格式，压缩率最高）
GET /uploads/images/example.jpg?f=avif

# 组合优化：300px宽度，80%质量，AVIF格式
GET /uploads/images/example.jpg?w=300&q=80&f=avif
```

### 特性说明

- **保持宽高比**: 只指定宽度或高度时自动保持比例
- **智能裁剪**: 同时指定宽高时使用Fit模式
- **格式转换**: 支持JPEG、PNG、WebP、**AVIF**格式输出
- **质量控制**: 可调整图片质量（1-100）
- **尺寸限制**: 受配置文件中max_width和max_height限制
- **缓存友好**: 优化后的图片设置长期缓存头
- **AVIF支持**: 支持最新的AVIF格式，提供更高的压缩率和更好的图片质量

---

## 📝 使用注意事项

1. **签名有效期**: 默认为 1 小时，可在配置文件中调整
2. **文件大小限制**: 默认最大 100MB，可在配置文件中调整
3. **文件类型控制**: 支持通配符配置（`*`、`image/*`、`video/*`等），灵活控制允许的文件类型
4. **AVIF格式支持**: 完整支持AVIF格式的上传、优化和缩略图生成
5. **缩略图生成**: 仅对图片文件生成，支持AVIF等现代格式，可配置生成条件
6. **图片优化**: 支持动态尺寸调整、质量控制和格式转换（包括AVIF）
7. **存储类型**: 支持本地存储和 S3 存储，可配置多个存储实例
8. **防盗链保护**: 可配置 Referer 检查和自定义防盗链图片

---

## 🔗 相关文档

- [调用说明.md](调用说明.md) - 详细的调用示例和客户端 SDK
- [Bun-Next.js调用流程.md](bun-next_js调用流程.md) - 前后端集成指南
- [Referer防盗链配置说明.md](Referer防盗链配置说明.md) - 防盗链配置
- [README.md](../README.md) - 项目主文档

---

*最后更新: 2025-01-07*
