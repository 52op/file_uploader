# 自动DNS记录管理注意事项

## 概述

本程序支持自动DNS记录管理功能，可以自动检测外网IP变化并更新DNS解析记录，支持动态IP环境。目前支持三大DNS服务商：阿里云DNS、腾讯云DNSPod、Cloudflare。

## 功能特性

- ✅ 自动检测外网IP（支持多个API源，自动故障转移）
- ✅ 自动创建/更新DNS A记录
- ✅ 定时检查DNS记录与IP匹配情况
- ✅ 支持多域名管理
- ✅ 与ACME证书申请无缝集成

## 配置说明

在 `config/config.yaml` 中配置：

```yaml
acme:
  dns:
    provider: "alidns"              # DNS提供商：alidns, tencentcloud, cloudflare
    auto_dns_record: true           # 启用自动DNS记录管理
    external_ip_apis:               # 外网IP检测API列表（按顺序尝试）
      - "https://api.ipify.org"
      - "https://ipv4.icanhazip.com"
      - "https://checkip.amazonaws.com"
      - "https://ipinfo.io/ip"
      - "https://api.myip.com"
    dns_check_interval: 60          # DNS检查间隔（分钟），0表示只在启动时检查
    config:
      # 根据不同提供商配置相应参数
```

---

## 阿里云DNS配置

### 1. 获取AccessKey

#### 方案A：主账号AccessKey（推荐用于测试）

1. 登录阿里云控制台
2. 点击右上角头像 → **AccessKey管理**
3. 点击**创建AccessKey**
4. 记录 `AccessKey ID` 和 `AccessKey Secret`

#### 方案B：RAM子用户AccessKey（推荐用于生产）

1. 登录阿里云控制台 → **访问控制RAM**
2. **用户管理** → **创建用户**
3. 勾选**编程访问**，创建用户
4. 记录生成的 `AccessKey ID` 和 `AccessKey Secret`

### 2. 配置权限（仅RAM子用户需要）

为RAM子用户添加DNS权限：

1. **RAM控制台** → **用户管理** → 找到创建的用户
2. 点击**添加权限**
3. 选择权限策略：

#### 系统策略（简单）：

- `AliyunDNSFullAccess` - DNS完全访问权限

#### 自定义策略（安全）：

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "alidns:AddDomainRecord",
        "alidns:DeleteDomainRecord", 
        "alidns:UpdateDomainRecord",
        "alidns:DescribeDomainRecords",
        "alidns:DescribeDomains"
      ],
      "Resource": "*"
    }
  ]
}
```

### 3. 配置文件示例

```yaml
acme:
  dns:
    provider: "alidns"
    auto_dns_record: true
    config:
      access_key_id: "key_id"
      access_key_secret: "key_secret"
      region: "cn-hangzhou"
```

### 4. 常见错误

**错误：`Forbidden.RAM`**

- **原因**：RAM子用户没有DNS操作权限
- **解决**：按上述步骤添加DNS权限，或使用主账号AccessKey

---

## 腾讯云DNSPod配置

### 1. 获取API密钥

#### 方案A：主账号API密钥

1. 登录腾讯云控制台
2. 点击右上角头像 → **访问管理**
3. **访问密钥** → **API密钥管理**
4. 点击**新建密钥**
5. 记录 `SecretId` 和 `SecretKey`

#### 方案B：子用户API密钥（推荐）

1. **访问管理** → **用户** → **用户列表**
2. 点击**新建用户** → **自定义创建**
3. 选择**可访问资源并接收消息**
4. 勾选**编程访问**
5. 记录生成的 `SecretId` 和 `SecretKey`

### 2. 配置权限（仅子用户需要）

为子用户添加DNSPod权限：

1. **用户列表** → 找到创建的用户 → **关联策略**
2. 选择策略：

#### 系统策略：

- `QcloudDNSPodFullAccess` - DNSPod完全访问权限

#### 自定义策略：

```json
{
    "version": "2.0",
    "statement": [
        {
            "effect": "allow",
            "action": [
                "dnspod:CreateRecord",
                "dnspod:DeleteRecord",
                "dnspod:ModifyRecord",
                "dnspod:DescribeRecordList",
                "dnspod:DescribeDomainList"
            ],
            "resource": "*"
        }
    ]
}
```

### 3. 配置文件示例

```yaml
acme:
  dns:
    provider: "tencentcloud"
    auto_dns_record: true
    config:
      secret_id: "xxxx"
      secret_key: "xxxx"
```

### 4. 注意事项

- 腾讯云DNSPod使用v20210323版本API
- 支持所有DNSPod托管的域名
- 默认使用"默认"线路，TTL为600秒

---

## Cloudflare配置

### 1. 获取API凭证

#### 方案A：API Token（推荐）

1. 登录Cloudflare控制台
2. 右上角头像 → **My Profile**
3. **API Tokens** → **Create Token**
4. 选择**Custom token**模板
5. 配置权限：
   - **Permissions**: `Zone:DNS:Edit`, `Zone:Zone:Read`
   - **Zone Resources**: `Include - All zones` 或指定域名
6. 点击**Continue to summary** → **Create Token**
7. 记录生成的Token

#### 方案B：Global API Key（不推荐）

1. **API Tokens** → **Global API Key** → **View**
2. 记录API Key和注册邮箱

### 2. 配置文件示例

#### 使用API Token（推荐）：

```yaml
acme:
  dns:
    provider: "cloudflare"
    auto_dns_record: true
    config:
      api_token: "your-api-token-here"
```

#### 使用Global API Key：

```yaml
acme:
  dns:
    provider: "cloudflare"
    auto_dns_record: true
    config:
      email: "your-email@example.com"
      api_key: "your-global-api-key"
```

### 3. 权限说明

API Token需要以下权限：

- **Zone:DNS:Edit** - 编辑DNS记录
- **Zone:Zone:Read** - 读取区域信息

### 4. 注意事项

- Cloudflare的DNS记录更新通常在几秒内生效
- 支持所有托管在Cloudflare的域名
- 默认TTL为1（自动）

---

## 故障排除

### 1. 权限问题

**症状**：出现 `Forbidden`、`Unauthorized` 等错误
**解决**：

- 检查API密钥是否正确
- 确认账号/子用户有相应DNS权限
- 尝试使用主账号密钥测试

### 2. 网络问题

**症状**：外网IP检测失败
**解决**：

- 检查网络连接
- 程序会自动尝试多个IP检测API
- 可在配置中添加更多IP检测源

### 3. DNS传播延迟

**症状**：DNS记录更新后仍解析到旧IP
**解决**：

- 等待DNS传播（通常5-30分钟）
- 使用 `nslookup` 或 `dig` 命令检查
- 清除本地DNS缓存

### 4. 域名不存在

**症状**：`no such host` 错误
**解决**：

- 确认域名已在DNS服务商处托管
- 检查域名拼写是否正确
- 确认域名已完成实名认证（国内域名）

---

## 最佳实践

1. **生产环境使用子用户/子账号**，遵循最小权限原则
2. **定期轮换API密钥**，提高安全性
3. **设置合理的检查间隔**，避免API调用过频
4. **监控日志输出**，及时发现问题
5. **备份重要DNS记录**，防止误操作

---

## 配置模板

完整的配置文件模板：

```yaml
server:
  https:
    enabled: true
    acme:
      enabled: true
      domains:
        - "your-domain.com"
        - "*.your-domain.com"  # 支持通配符
      dns:
        provider: "alidns"  # 或 tencentcloud, cloudflare
        auto_dns_record: true
        external_ip_apis:
          - "https://api.ipify.org"
          - "https://ipv4.icanhazip.com"
          - "https://checkip.amazonaws.com"
        dns_check_interval: 60  # 分钟
        config:
          # 根据选择的provider配置相应参数
          access_key_id: "your-key"
          access_key_secret: "your-secret"
```

启用此功能后，程序将自动：

1. 检测外网IP变化
2. 更新DNS A记录
3. 申请/续期SSL证书
4. 定期检查并维护DNS记录

这样就实现了真正的"零配置"动态域名解析服务！

---

## 高级配置

### 1. 多域名配置

```yaml
acme:
  domains:
    - "api.example.com"
    - "cdn.example.com"
    - "upload.example.com"
  dns:
    auto_dns_record: true
    # 所有域名都会自动创建A记录指向当前外网IP
```

### 2. 自定义外网IP检测

```yaml
acme:
  dns:
    external_ip_apis:
      - "https://api.ipify.org"
      - "https://ipv4.icanhazip.com"
      - "https://checkip.amazonaws.com"
      - "https://ipinfo.io/ip"
      - "https://api.myip.com"
      - "https://ifconfig.me/ip"  # 添加更多API源
    # external_ip: "1.2.3.4"  # 手动指定IP（跳过自动检测）
```

### 3. 检查间隔配置

```yaml
acme:
  dns:
    dns_check_interval: 0    # 只在启动时检查
    # dns_check_interval: 30   # 每30分钟检查一次
    # dns_check_interval: 1440 # 每24小时检查一次
```

---

## API密钥安全建议

### 1. 环境变量方式（推荐）

```bash
# 设置环境变量
export ALIYUN_ACCESS_KEY_ID="your-key-id"
export ALIYUN_ACCESS_KEY_SECRET="your-key-secret"
export TENCENT_SECRET_ID="your-secret-id"
export TENCENT_SECRET_KEY="your-secret-key"
export CLOUDFLARE_API_TOKEN="your-api-token"
```

然后在配置文件中引用：

```yaml
acme:
  dns:
    config:
      access_key_id: "${ALIYUN_ACCESS_KEY_ID}"
      access_key_secret: "${ALIYUN_ACCESS_KEY_SECRET}"
```

### 2. 文件权限设置

```bash
# 设置配置文件权限（仅所有者可读写）
chmod 600 config/config.yaml
```

### 3. 密钥轮换

- 定期更换API密钥（建议3-6个月）
- 删除不再使用的旧密钥
- 监控API密钥使用情况

---

## 监控和日志

### 1. 日志级别

程序会输出详细的操作日志：

```
[DNS管理] 开始检查DNS记录，域名数量: 2
[IP检测] 成功检测到外网IP: 1.2.3.4 (来源: https://api.ipify.org)
[DNS管理] 检查域名: api.example.com
[DNS管理] DNS记录已是最新: api.example.com -> 1.2.3.4
[阿里云DNS] 更新A记录: cdn.example.com -> 1.2.3.4
[DNS管理] 成功更新DNS记录: cdn.example.com
```

### 2. 错误处理

程序具有完善的错误处理机制：

- API调用失败会自动重试
- 外网IP检测支持多源故障转移
- DNS更新失败不会影响其他功能

### 3. 状态监控

可通过API端点监控DNS管理状态：

```bash
# 获取ACME状态（包含DNS管理信息）
curl "http://localhost:8080/debug/acme"
```

---

## 与其他功能的集成

### 1. ACME证书申请

自动DNS记录管理与ACME证书申请完美集成：

1. 程序启动时检查DNS记录
2. 确保域名解析到当前服务器
3. 申请Let's Encrypt证书
4. 定期检查IP变化并更新DNS
5. 自动续期证书

### 2. 负载均衡支持

```yaml
# 多服务器部署时，每台服务器可配置不同的子域名
server1: api1.example.com
server2: api2.example.com
# 主域名通过负载均衡器分发流量
```

### 3. 容器化部署

```dockerfile
# Dockerfile示例
FROM golang:alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o file_uploader

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/file_uploader .
COPY --from=builder /app/config ./config
CMD ["./file_uploader"]
```

---

## 常见问题FAQ

### Q1: 支持IPv6吗？

A: 目前只支持IPv4，IPv6支持在开发计划中。

### Q2: 可以管理CNAME记录吗？

A: 目前只支持A记录，其他记录类型支持在开发计划中。

### Q3: 支持多个DNS服务商同时使用吗？

A: 目前一次只能配置一个DNS服务商，多服务商支持在开发计划中。

### Q4: DNS检查会影响性能吗？

A: DNS检查在后台异步执行，不会影响主要功能性能。

### Q5: 如何备份DNS记录？

A: 建议在DNS服务商控制台手动备份重要记录，或使用专门的DNS备份工具。

---

## 技术支持

如遇到问题，请提供以下信息：

1. 完整的错误日志
2. 配置文件（隐藏敏感信息）
3. DNS服务商和域名信息
4. 网络环境描述

联系方式：

- GitHub Issues: [项目地址]
- 邮箱: [支持邮箱]

---

**最后更新**: 2025年7月4日
**版本**: v1.0.0
