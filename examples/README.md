# 文件上传服务客户端工具

本目录包含两个实用的客户端工具，用于与文件上传服务进行交互和测试。

## 工具列表

### 1. client_signature.exe - 文件上传客户端
功能完整的命令行客户端，支持文件上传、下载、删除等操作。

### 2. generate_static_signature.exe - 静态文件签名生成器
用于生成访问受保护静态文件的签名URL。

---

## 🚀 client_signature.exe

### 功能特性

- ✅ **健康检查** - 检查服务器状态
- ✅ **文件上传** - 支持多种存储类型和自定义路径
- ✅ **文件信息** - 获取已上传文件的详细信息
- ✅ **文件删除** - 删除指定文件
- ✅ **命令行参数** - 灵活的参数配置
- ✅ **多存储支持** - 支持不同的存储后端

### 使用方法

```bash
client_signature.exe [全局选项] <命令> [命令参数]
```

### 全局选项

| 选项 | 长选项 | 说明 | 默认值 |
|------|--------|------|--------|
| `-u` | `--url` | 服务器地址 | `http://localhost:8080` |
| `-k` | `--key` | 签名密钥 | `your-secret-key-change-this-in-production` |
| `-s` | `--storage` | 存储类型 | 无（使用默认存储） |
| `-h` | `--help` | 显示帮助信息 | - |

### 命令详解

#### 1. health - 健康检查
检查服务器是否正常运行。

```bash
# 基本用法
client_signature.exe health

# 指定服务器
client_signature.exe -u https://your-server:8443 health
```

#### 2. upload - 文件上传
上传文件到服务器。

```bash
# 基本上传（默认存储）
client_signature.exe -u https://your-server:8443 -k your-secret-key upload file.txt

# 上传到指定存储
client_signature.exe -u https://your-server:8443 -k your-secret-key -s local_backup upload file.txt

# 上传到自定义路径
client_signature.exe -u https://your-server:8443 -k your-secret-key -s local_backup -p /custom/path/file.txt upload file.txt
```

**upload 命令专用选项：**
- `-p, --path` - 自定义文件路径

#### 3. info - 获取文件信息
获取已上传文件的详细信息。

```bash
# 获取默认存储中的文件信息
client_signature.exe -u https://your-server:8443 -k your-secret-key info filename.txt

# 获取指定存储中的文件信息
client_signature.exe -u https://your-server:8443 -k your-secret-key -s local_backup info filename.txt
```

#### 4. delete - 删除文件
删除服务器上的文件。

```bash
# 删除默认存储中的文件
client_signature.exe -u https://your-server:8443 -k your-secret-key delete filename.txt

# 删除指定存储中的文件
client_signature.exe -u https://your-server:8443 -k your-secret-key -s local_backup delete filename.txt
```

### 实际使用示例

```bash
# 1. 检查服务器状态
client_signature.exe -u https://wuhu-cdn.hxljzz.com:8443 health

# 2. 上传文件到默认存储（公开访问）
client_signature.exe -u https://wuhu-cdn.hxljzz.com:8443 -k your-secret-key-change-this-in-production upload test.jpg

# 3. 上传文件到备份存储（需要签名访问）
client_signature.exe -u https://wuhu-cdn.hxljzz.com:8443 -k your-secret-key-change-this-in-production -s local_backup upload document.pdf

# 4. 上传到自定义路径
client_signature.exe -u https://wuhu-cdn.hxljzz.com:8443 -k your-secret-key-change-this-in-production -s local_backup -p /docs/2025/report.pdf upload report.pdf

# 5. 获取文件信息
client_signature.exe -u https://wuhu-cdn.hxljzz.com:8443 -k your-secret-key-change-this-in-production info 20250704_120000_test.jpg

# 6. 删除文件
client_signature.exe -u https://wuhu-cdn.hxljzz.com:8443 -k your-secret-key-change-this-in-production delete 20250704_120000_test.jpg
```

---

## 🔐 generate_static_signature.exe

### 功能特性

- ✅ **签名生成** - 为受保护的静态文件生成访问签名
- ✅ **URL构建** - 自动构建完整的签名访问URL
- ✅ **时间显示** - 显示签名过期时间
- ✅ **测试命令** - 提供ready-to-use的curl测试命令

### 使用方法

```bash
generate_static_signature.exe <文件路径> [密钥]
```

### 参数说明

- `<文件路径>` - 要访问的文件路径（必需）
- `[密钥]` - 签名密钥（可选，默认使用标准密钥）

### 使用示例

```bash
# 使用默认密钥生成签名
generate_static_signature.exe "/backup/test/2025/document.pdf"

# 使用自定义密钥生成签名
generate_static_signature.exe "/backup/test/2025/document.pdf" "your-custom-secret-key"
```

### 输出示例

```
文件路径: /backup/test/2025/document.pdf
过期时间: 1751642573 (2025-07-04 23:22:53)
签名: 8f23092b83f0a1d35591beaf146025cdb5520cfadc8ae081175f3e2212f4cb0256a36736f9719225
完整URL: https://wuhu-cdn.hxljzz.com:8443/backup/test/2025/document.pdf?expires=1751642573&signature=8f23092b83f0a1d35591beaf146025cdb5520cfadc8ae081175f3e2212f4cb0256a36736f9719225

测试命令:
curl "https://wuhu-cdn.hxljzz.com:8443/backup/test/2025/document.pdf?expires=1751642573&signature=8f23092b83f0a1d35591beaf146025cdb5520cfadc8ae081175f3e2212f4cb0256a36736f9719225"
```

### 使用场景

1. **测试受保护文件访问** - 验证混合访问控制是否正常工作
2. **调试签名问题** - 生成正确的签名进行对比
3. **临时文件分享** - 为受保护文件生成临时访问链接
4. **API集成测试** - 在开发过程中快速生成测试URL

---

## 📋 编译说明

如果需要重新编译这些工具：

```bash
# 编译文件上传客户端
go build -o client_signature.exe client_signature.go

# 编译签名生成器
go build -o generate_static_signature.exe generate_static_signature.go
```

## 🔧 配置说明

### 服务器地址配置
工具默认连接到 `https://wuhu-cdn.hxljzz.com:8443`，你可以通过 `-u` 参数指定其他服务器。

### 密钥配置
默认使用 `your-secret-key-change-this-in-production`，请确保与服务器配置一致。

### 存储类型
支持的存储类型取决于服务器配置，常见的有：
- `default` - 默认存储（通常是公开访问）
- `local_backup` - 本地备份存储（通常需要签名验证）
- `s3_backup` - S3备份存储
- 其他自定义存储名称

## 🚨 注意事项

1. **密钥安全** - 不要在命令行历史中暴露真实的密钥
2. **签名有效期** - 生成的签名默认1小时有效
3. **网络连接** - 确保能够访问目标服务器
4. **文件路径** - 路径格式需要与服务器存储配置匹配

## 📞 故障排除

### 常见错误

1. **连接失败** - 检查服务器地址和网络连接
2. **签名验证失败** - 确认密钥是否正确
3. **文件不存在** - 确认文件路径和存储类型
4. **权限拒绝** - 检查是否需要签名访问

### 调试技巧

1. 先使用 `health` 命令测试连接
2. 使用 `generate_static_signature.exe` 验证签名生成
3. 检查服务器日志获取详细错误信息
4. 确认文件确实存在于指定存储中

## 🎯 快速开始

### 第一次使用

1. **健康检查**
   ```bash
   client_signature.exe -u https://your-server:8443 health
   ```

2. **上传测试文件**
   ```bash
   echo "Hello World" > test.txt
   client_signature.exe -u https://your-server:8443 -k your-secret-key upload test.txt
   ```

3. **访问上传的文件**
   ```bash
   # 如果是公开存储，直接访问
   curl "https://your-server:8443/uploads/20250704_120000_test.txt"

   # 如果是受保护存储，需要生成签名
   generate_static_signature.exe "/backup/20250704_120000_test.txt"
   ```

### 批量操作示例

```bash
# 批量上传多个文件到备份存储
for file in *.jpg; do
    client_signature.exe -u https://your-server:8443 -k your-secret-key -s local_backup upload "$file"
done

# 批量生成签名（PowerShell）
Get-ChildItem *.pdf | ForEach-Object {
    generate_static_signature.exe "/backup/$($_.Name)"
}
```

## 📊 混合访问控制说明

文件上传服务支持混合访问控制，不同存储可以有不同的访问策略：

### 公开访问存储
- **特点**: 无需签名，直接访问
- **适用**: 图片、CSS、JS等公开资源
- **访问**: `https://server/uploads/filename`

### 签名验证存储
- **特点**: 需要有效签名才能访问
- **适用**: 用户文档、私有数据等敏感文件
- **访问**: `https://server/backup/filename?expires=xxx&signature=xxx`

### 配置示例
```yaml
# 服务器配置示例
storage:
  storages:
    public_images:      # 公开图片存储
      type: local
      upload_dir: ./images
      # 不设置 require_auth，使用全局默认（公开访问）

    private_docs:       # 私有文档存储
      type: local
      upload_dir: ./docs
      require_auth: true  # 需要签名验证
```

## 🔄 工作流程示例

### 内容管理系统工作流
```bash
# 1. 上传公开图片
client_signature.exe -u https://cms.example.com:8443 -k secret-key -s public_images upload logo.png

# 2. 上传私有文档
client_signature.exe -u https://cms.example.com:8443 -k secret-key -s private_docs upload contract.pdf

# 3. 为私有文档生成临时访问链接
generate_static_signature.exe "/docs/contract.pdf" secret-key

# 4. 清理过期文件
client_signature.exe -u https://cms.example.com:8443 -k secret-key -s private_docs delete old_contract.pdf
```

### 开发测试工作流
```bash
# 1. 检查服务状态
client_signature.exe -u https://dev-server:8443 health

# 2. 上传测试文件
client_signature.exe -u https://dev-server:8443 -k dev-key upload test_data.json

# 3. 验证文件可访问性
curl "https://dev-server:8443/uploads/20250704_120000_test_data.json"

# 4. 测试受保护存储
client_signature.exe -u https://dev-server:8443 -k dev-key -s secure_storage upload sensitive.txt
generate_static_signature.exe "/secure/sensitive.txt" dev-key
```

---

**开发者**: 文件上传服务团队
**更新时间**: 2025-07-04
**版本**: v1.0.0

> 💡 **提示**: 这些工具是开源的，源代码位于同目录下的 `.go` 文件中，你可以根据需要进行修改和扩展。
