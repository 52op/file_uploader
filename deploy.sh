#!/bin/bash

# 文件上传服务部署脚本
# 支持多种部署方式：本地编译、Docker、系统服务

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

# 检查命令是否存在
check_command() {
    if ! command -v $1 &> /dev/null; then
        print_error "$1 命令未找到，请先安装"
        exit 1
    fi
}

# 显示帮助信息
show_help() {
    echo "文件上传服务部署脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  build           编译应用程序"
    echo "  docker          使用Docker部署"
    echo "  docker-compose  使用Docker Compose部署"
    echo "  service         安装为系统服务"
    echo "  clean           清理构建文件"
    echo "  help            显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 build                # 编译应用程序"
    echo "  $0 docker              # Docker部署"
    echo "  $0 docker-compose      # Docker Compose部署"
    echo "  $0 service             # 安装系统服务"
}

# 编译应用程序
build_app() {
    print_info "开始编译应用程序..."
    
    # 检查Go环境
    check_command go
    
    # 清理之前的构建
    rm -f file_uploader file_uploader.exe
    
    # 编译
    print_info "编译中..."
    go mod tidy
    go build -ldflags "-s -w" -o file_uploader .
    
    # 检查编译结果
    if [ -f "file_uploader" ]; then
        print_success "编译完成: file_uploader"
        
        # 显示文件信息
        ls -lh file_uploader
        
        # 创建配置文件（如果不存在）
        if [ ! -f "config/config.yaml" ]; then
            print_info "创建默认配置文件..."
            cp config/config.example.yaml config/config.yaml
            print_warning "请编辑 config/config.yaml 文件以配置您的服务"
        fi
        
        print_success "构建完成！运行 './file_uploader' 启动服务"
    else
        print_error "编译失败"
        exit 1
    fi
}

# Docker部署
deploy_docker() {
    print_info "开始Docker部署..."
    
    # 检查Docker环境
    check_command docker
    
    # 构建Docker镜像
    print_info "构建Docker镜像..."
    docker build -t file_uploader:latest .
    
    # 停止并删除旧容器
    print_info "停止旧容器..."
    docker stop file_uploader 2>/dev/null || true
    docker rm file_uploader 2>/dev/null || true
    
    # 创建上传目录
    mkdir -p uploads
    
    # 创建配置文件（如果不存在）
    if [ ! -f "config/config.yaml" ]; then
        print_info "创建默认配置文件..."
        cp config/config.example.yaml config/config.yaml
        print_warning "请编辑 config/config.yaml 文件以配置您的服务"
    fi
    
    # 运行新容器
    print_info "启动新容器..."
    docker run -d \
        --name file_uploader \
        -p 8080:8080 \
        -v $(pwd)/uploads:/app/uploads \
        -v $(pwd)/config/config.yaml:/app/config/config.yaml:ro \
        --restart unless-stopped \
        file_uploader:latest
    
    print_success "Docker部署完成！"
    print_info "容器状态:"
    docker ps | grep file_uploader
    
    print_info "查看日志: docker logs -f file_uploader"
}

# Docker Compose部署
deploy_docker_compose() {
    print_info "开始Docker Compose部署..."
    
    # 检查Docker Compose环境
    check_command docker-compose
    
    # 创建配置文件（如果不存在）
    if [ ! -f "config/config.yaml" ]; then
        print_info "创建默认配置文件..."
        cp config/config.example.yaml config/config.yaml
        print_warning "请编辑 config/config.yaml 文件以配置您的服务"
    fi
    
    # 停止旧服务
    print_info "停止旧服务..."
    docker-compose down 2>/dev/null || true
    
    # 启动服务
    print_info "启动服务..."
    docker-compose up -d --build
    
    print_success "Docker Compose部署完成！"
    print_info "服务状态:"
    docker-compose ps
    
    print_info "查看日志: docker-compose logs -f"
}

# 安装系统服务
install_service() {
    print_info "安装系统服务..."
    
    # 检查是否为root用户
    if [ "$EUID" -ne 0 ]; then
        print_error "请使用root权限运行此命令"
        exit 1
    fi
    
    # 编译应用程序
    build_app
    
    # 创建服务目录
    SERVICE_DIR="/opt/file_uploader"
    print_info "创建服务目录: $SERVICE_DIR"
    mkdir -p $SERVICE_DIR
    
    # 复制文件
    cp file_uploader $SERVICE_DIR/
    cp -r config $SERVICE_DIR/
    mkdir -p $SERVICE_DIR/uploads
    
    # 设置权限
    chown -R www-data:www-data $SERVICE_DIR
    chmod +x $SERVICE_DIR/file_uploader
    
    # 创建systemd服务文件
    print_info "创建systemd服务文件..."
    cat > /etc/systemd/system/file_uploader.service << EOF
[Unit]
Description=File Uploader Service
After=network.target

[Service]
Type=simple
User=www-data
Group=www-data
WorkingDirectory=$SERVICE_DIR
ExecStart=$SERVICE_DIR/file_uploader
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    
    # 重新加载systemd
    systemctl daemon-reload
    
    # 启用并启动服务
    systemctl enable file_uploader
    systemctl start file_uploader
    
    print_success "系统服务安装完成！"
    print_info "服务状态:"
    systemctl status file_uploader --no-pager
    
    print_info "管理命令:"
    print_info "  启动服务: systemctl start file_uploader"
    print_info "  停止服务: systemctl stop file_uploader"
    print_info "  重启服务: systemctl restart file_uploader"
    print_info "  查看状态: systemctl status file_uploader"
    print_info "  查看日志: journalctl -u file_uploader -f"
}

# 清理构建文件
clean_build() {
    print_info "清理构建文件..."
    
    rm -f file_uploader file_uploader.exe
    docker rmi file_uploader:latest 2>/dev/null || true
    
    print_success "清理完成"
}

# 主函数
main() {
    case "${1:-help}" in
        build)
            build_app
            ;;
        docker)
            deploy_docker
            ;;
        docker-compose)
            deploy_docker_compose
            ;;
        service)
            install_service
            ;;
        clean)
            clean_build
            ;;
        help|*)
            show_help
            ;;
    esac
}

# 执行主函数
main "$@"
