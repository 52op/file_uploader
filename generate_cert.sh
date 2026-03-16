#!/bin/bash

# 生成自签名SSL证书脚本
# 用于开发和测试环境

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查openssl命令
check_openssl() {
    if ! command -v openssl &> /dev/null; then
        print_error "openssl 命令未找到，请先安装 OpenSSL"
        exit 1
    fi
}

# 生成自签名证书
generate_cert() {
    local domain=${1:-localhost}
    local cert_file="cert.pem"
    local key_file="key.pem"
    local days=365
    
    print_info "开始生成自签名SSL证书..."
    print_info "域名: $domain"
    print_info "有效期: $days 天"
    print_info "证书文件: $cert_file"
    print_info "私钥文件: $key_file"
    
    # 检查文件是否已存在
    if [ -f "$cert_file" ] || [ -f "$key_file" ]; then
        print_warning "证书文件已存在"
        read -p "是否覆盖现有证书? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_info "取消生成证书"
            exit 0
        fi
        rm -f "$cert_file" "$key_file"
    fi
    
    # 创建临时配置文件
    local config_file=$(mktemp)
    cat > "$config_file" << EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
C=CN
ST=Beijing
L=Beijing
O=File Uploader
OU=Development
CN=$domain

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = $domain
DNS.2 = localhost
DNS.3 = *.localhost
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

    # 生成私钥和证书
    print_info "生成私钥..."
    openssl genrsa -out "$key_file" 2048
    
    print_info "生成证书..."
    openssl req -new -x509 -key "$key_file" -out "$cert_file" -days "$days" -config "$config_file"
    
    # 清理临时文件
    rm -f "$config_file"
    
    # 设置文件权限
    chmod 600 "$key_file"
    chmod 644 "$cert_file"
    
    print_success "SSL证书生成完成！"
    print_info "证书信息:"
    openssl x509 -in "$cert_file" -text -noout | grep -E "(Subject:|DNS:|IP Address:|Not Before|Not After)"
    
    print_info ""
    print_info "使用方法:"
    print_info "1. 修改 config/config.yaml 中的 HTTPS 配置:"
    print_info "   https:"
    print_info "     enabled: true"
    print_info "     cert_file: \"cert.pem\""
    print_info "     key_file: \"key.pem\""
    print_info "     port: \"8443\""
    print_info ""
    print_info "2. 启动服务器后访问: https://localhost:8443"
    print_info ""
    print_warning "注意: 这是自签名证书，浏览器会显示安全警告"
    print_warning "生产环境请使用正式的SSL证书"
}

# 显示帮助信息
show_help() {
    echo "SSL证书生成脚本"
    echo ""
    echo "用法: $0 [域名]"
    echo ""
    echo "参数:"
    echo "  域名    可选，默认为 localhost"
    echo ""
    echo "示例:"
    echo "  $0                    # 生成 localhost 证书"
    echo "  $0 example.com        # 生成 example.com 证书"
    echo "  $0 myapp.local        # 生成 myapp.local 证书"
    echo ""
    echo "生成的文件:"
    echo "  cert.pem - SSL证书文件"
    echo "  key.pem  - SSL私钥文件"
}

# 主函数
main() {
    case "${1:-}" in
        -h|--help|help)
            show_help
            ;;
        *)
            check_openssl
            generate_cert "$1"
            ;;
    esac
}

# 执行主函数
main "$@"
