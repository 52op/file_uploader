# Referer 防盗链配置说明

## 📋 功能概述

本文件上传服务支持基于 Referer 头的防盗链保护，可以有效防止其他网站直接引用您的图片、视频等静态资源，节省带宽成本并保护内容版权。

## 🔧 配置方式

### 1. 基本配置结构

在 `config.yaml` 中为每个存储单独配置 `allow_referer` 字段：

```yaml
storage:
  storages:
    storage_name:
      type: local
      upload_dir: ./uploads
      base_url: https://your-domain.com/uploads
      allow_referer:           # Referer白名单配置
        - "https://your-domain.com"
        - "https://www.your-domain.com"
        - "blank"              # 允许空referer（直接访问）
        - "localhost"          # 开发环境
```

### 2. 配置选项说明

| 配置项 | 说明 | 示例 |
|--------|------|------|
| `allow_referer` | Referer白名单列表 | 见下方详细说明 |
| 不配置 `allow_referer` | 不进行Referer检查，允许所有访问 | - |
| 空数组 `[]` | 拒绝所有访问 | `allow_referer: []` |

### 3. Referer 匹配规则

#### 3.1 完整URL匹配
```yaml
allow_referer:
  - "https://example.com/gallery"  # 精确匹配完整URL
```

#### 3.2 域名匹配（带协议）
```yaml
allow_referer:
  - "https://example.com"          # 匹配该域名下所有HTTPS页面
  - "http://example.com"           # 匹配该域名下所有HTTP页面
```

#### 3.3 域名匹配（不带协议）
```yaml
allow_referer:
  - "example.com"                  # 匹配该域名（HTTP和HTTPS）
  - "www.example.com"              # 匹配指定子域名
```

#### 3.4 子域名通配符匹配
```yaml
allow_referer:
  - ".example.com"                 # 匹配 example.com 及其所有子域名
```

#### 3.5 特殊值
```yaml
allow_referer:
  - "blank"                        # 允许空referer（直接访问、书签访问）
  - "localhost"                    # 开发环境，匹配所有localhost
```

## 📝 配置示例

### 示例1：严格防盗链
```yaml
storage:
  storages:
    protected_images:
      type: local
      upload_dir: ./protected
      base_url: https://cdn.example.com/protected
      allow_referer:
        - "https://www.example.com"
        - "https://app.example.com"
        # 不包含 "blank"，禁止直接访问
```

### 示例2：允许直接访问的防盗链
```yaml
storage:
  storages:
    gallery:
      type: local
      upload_dir: ./gallery
      base_url: https://cdn.example.com/gallery
      allow_referer:
        - "https://www.example.com"
        - ".example.com"           # 允许所有子域名
        - "blank"                  # 允许直接访问
        - "localhost"              # 开发环境
```

### 示例3：开发和生产环境
```yaml
storage:
  storages:
    assets:
      type: local
      upload_dir: ./assets
      base_url: https://cdn.example.com/assets
      allow_referer:
        - "https://example.com"
        - "https://www.example.com"
        - "https://staging.example.com"
        - "localhost"              # 本地开发
        - "127.0.0.1"              # 本地开发
        - "blank"                  # 允许直接访问
```

### 示例4：公开存储（无防盗链）
```yaml
storage:
  storages:
    public_files:
      type: local
      upload_dir: ./public
      base_url: https://cdn.example.com/public
      # 不配置 allow_referer，表示不进行检查
```

## 🚫 防盗链响应

当 Referer 检查失败时，系统会根据请求的文件类型返回不同的防盗链内容：

### 图片文件 (.jpg, .png, .gif 等)
- 返回 1x1 透明PNG图片
- 或返回自定义防盗链图片（如果存在 `./static/anti-hotlink.jpg`）

### 视频文件 (.mp4, .avi, .mov 等)
- 返回 JSON 错误信息
- HTTP 状态码：403 Forbidden

### 其他文件
- 返回防盗链HTML页面
- 包含友好的错误提示和配置说明

## 📊 日志记录

防盗链拦截会记录详细日志：

```
[ANTI-HOTLINK] 阻止盗链访问 - IP: 192.168.1.100, Referer: https://evil-site.com, Path: /gallery/image.jpg, UserAgent: Mozilla/5.0..., Storage: gallery
```

## 🧪 测试方法

### 1. 使用 PowerShell 测试
```powershell
# 测试无Referer访问
Invoke-WebRequest -Uri "https://your-domain.com/uploads/test.jpg" -Method GET

# 测试带Referer访问
$headers = @{ 'Referer' = 'https://allowed-domain.com' }
Invoke-WebRequest -Uri "https://your-domain.com/uploads/test.jpg" -Headers $headers

# 测试恶意Referer
$headers = @{ 'Referer' = 'https://evil-site.com' }
Invoke-WebRequest -Uri "https://your-domain.com/uploads/test.jpg" -Headers $headers
```

### 2. 使用 curl 测试
```bash
# 无Referer
curl -I "https://your-domain.com/uploads/test.jpg"

# 允许的Referer
curl -H "Referer: https://allowed-domain.com" -I "https://your-domain.com/uploads/test.jpg"

# 恶意Referer
curl -H "Referer: https://evil-site.com" -I "https://your-domain.com/uploads/test.jpg"
```

## ⚠️ 注意事项

### 1. Referer 头的限制
- 某些浏览器或代理可能会移除或修改 Referer 头
- HTTPS 页面引用 HTTP 资源时，Referer 可能为空
- 用户可以通过浏览器设置禁用 Referer

### 2. 安全性考虑
- Referer 检查只能防止简单的盗链，不能防止专业的爬虫
- 建议结合签名验证使用，提供更强的安全保护
- 对于高价值内容，建议使用水印等额外保护措施

### 3. 性能影响
- Referer 检查的性能开销很小
- 比签名验证更轻量级
- 适合大量静态文件的场景

## 🔄 与签名验证的配合

可以同时启用 Referer 检查和签名验证：

```yaml
storage:
  storages:
    secure_storage:
      type: local
      upload_dir: ./secure
      base_url: https://cdn.example.com/secure
      require_auth: true         # 启用签名验证
      allow_referer:             # 同时启用Referer检查
        - "https://app.example.com"
        - "blank"
```

执行顺序：Referer 检查 → 签名验证 → 文件服务

## 📈 最佳实践

1. **分层保护**：根据内容价值选择不同的保护级别
2. **白名单管理**：定期审查和更新 Referer 白名单
3. **监控日志**：关注防盗链拦截日志，识别异常访问模式
4. **测试验证**：部署后进行充分测试，确保正常用户不受影响
5. **备用方案**：为重要内容准备多种防护措施
