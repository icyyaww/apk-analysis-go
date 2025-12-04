#!/bin/bash

# ============================================
# APK 动态分析平台 - 一键安装脚本
# ============================================
#
# 使用方法:
#   curl -fsSL https://raw.githubusercontent.com/icyyaww/apk-analysis-go/main/scripts/install.sh | bash
#
# 或者:
#   wget -qO- https://raw.githubusercontent.com/icyyaww/apk-analysis-go/main/scripts/install.sh | bash
#
# ============================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查命令是否存在
check_command() {
    if ! command -v $1 &> /dev/null; then
        return 1
    fi
    return 0
}

# 打印横幅
print_banner() {
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║       APK 动态分析平台 - 一键安装脚本                      ║${NC}"
    echo -e "${GREEN}║       APK Dynamic Analysis Platform - Installer            ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

# 检查系统要求
check_requirements() {
    log_info "检查系统要求..."

    # 检查操作系统
    if [[ "$OSTYPE" != "linux-gnu"* ]]; then
        log_error "此脚本仅支持 Linux 系统"
        exit 1
    fi

    # 检查 Docker
    if ! check_command docker; then
        log_error "未安装 Docker，请先安装 Docker"
        log_info "安装命令: curl -fsSL https://get.docker.com | sh"
        exit 1
    fi

    # 检查 Docker Compose
    if ! check_command docker-compose && ! docker compose version &> /dev/null; then
        log_error "未安装 Docker Compose，请先安装"
        exit 1
    fi

    # 检查 Docker 服务是否运行
    if ! docker info &> /dev/null; then
        log_error "Docker 服务未运行，请启动 Docker"
        log_info "启动命令: sudo systemctl start docker"
        exit 1
    fi

    # 检查 Git
    if ! check_command git; then
        log_error "未安装 Git，请先安装"
        log_info "安装命令: sudo apt-get install -y git"
        exit 1
    fi

    log_success "系统要求检查通过"
}

# 克隆项目
clone_project() {
    log_info "克隆项目代码..."

    INSTALL_DIR="${INSTALL_DIR:-$HOME/apk-analysis-go}"

    if [ -d "$INSTALL_DIR" ]; then
        log_warn "目录 $INSTALL_DIR 已存在"
        read -p "是否删除并重新克隆? (y/n): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            rm -rf "$INSTALL_DIR"
        else
            log_info "使用已存在的目录"
            cd "$INSTALL_DIR"
            git pull origin main || true
            return
        fi
    fi

    git clone https://github.com/icyyaww/apk-analysis-go.git "$INSTALL_DIR"
    cd "$INSTALL_DIR"

    log_success "项目克隆完成: $INSTALL_DIR"
}

# 创建配置文件
create_config() {
    log_info "创建配置文件..."

    # 创建 .env 文件
    if [ ! -f ".env" ]; then
        cp .env.example .env
        log_info "已创建 .env 文件，请根据需要修改配置"
    else
        log_warn ".env 文件已存在，跳过创建"
    fi

    # 创建 config.yaml 文件
    if [ ! -f "configs/config.yaml" ]; then
        cp configs/config.yaml.example configs/config.yaml
        log_info "已创建 configs/config.yaml 文件"

        # 生成随机密码
        MYSQL_PASS=$(openssl rand -base64 12 | tr -dc 'a-zA-Z0-9' | head -c 16)
        RABBITMQ_PASS=$(openssl rand -base64 12 | tr -dc 'a-zA-Z0-9' | head -c 16)

        # 更新配置文件中的数据库连接（使用 Docker 内部网络）
        sed -i "s/host: your-database-host.com/host: mysql/" configs/config.yaml
        sed -i "s/password: your_database_password/password: $MYSQL_PASS/" configs/config.yaml
        sed -i "s/password: pass/password: $RABBITMQ_PASS/" configs/config.yaml

        # 更新 .env 文件
        sed -i "s/MYSQL_PASS=.*/MYSQL_PASS=$MYSQL_PASS/" .env
        sed -i "s/MYSQL_ROOT_PASSWORD=.*/MYSQL_ROOT_PASSWORD=$MYSQL_PASS/" .env
        sed -i "s/RABBITMQ_PASS=.*/RABBITMQ_PASS=$RABBITMQ_PASS/" .env

        log_success "配置文件已生成（使用随机密码）"
    else
        log_warn "configs/config.yaml 文件已存在，跳过创建"
    fi

    # 创建必要的目录
    mkdir -p inbound_apks results data logs

    log_success "配置文件创建完成"
}

# 创建 Docker Compose 覆盖文件（用于本地安装）
create_docker_compose_override() {
    log_info "创建 Docker Compose 配置..."

    # 如果没有外部数据库，创建包含 MySQL 的 compose 文件
    cat > docker-compose.local.yml << 'EOF'
version: '3.8'

services:
  # MySQL 数据库
  mysql:
    image: mysql:8.0
    container_name: apk-analysis-mysql
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_PASS:-apk_analysis_pass}
      MYSQL_DATABASE: apk_analysis_go
      MYSQL_USER: apk_user
      MYSQL_PASSWORD: ${MYSQL_PASS:-apk_analysis_pass}
    volumes:
      - mysql_data:/var/lib/mysql
    ports:
      - "3306:3306"
    networks:
      - apk-network
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Redis 缓存
  redis:
    image: redis:7-alpine
    container_name: apk-analysis-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    networks:
      - apk-network
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  # RabbitMQ 消息队列
  rabbitmq:
    image: rabbitmq:3-management-alpine
    container_name: apk-analysis-rabbitmq
    restart: unless-stopped
    environment:
      RABBITMQ_DEFAULT_USER: user
      RABBITMQ_DEFAULT_PASS: ${RABBITMQ_PASS:-pass}
    ports:
      - "5672:5672"
      - "15672:15672"
    networks:
      - apk-network
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "check_running"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Mitmproxy 流量代理
  mitmproxy-1:
    image: mitmproxy/mitmproxy:latest
    container_name: apk-analysis-mitmproxy-1
    restart: unless-stopped
    command: >
      mitmdump
      --mode regular
      --listen-port 8080
      --set block_global=false
      -s /app/scripts/mitm_jsonl_writer.py
    volumes:
      - ./scripts/mitm_jsonl_writer.py:/app/scripts/mitm_jsonl_writer.py:ro
      - ./results:/app/results
      - mitmproxy_certs:/home/mitmproxy/.mitmproxy
    ports:
      - "8082:8080"
      - "8083:8081"
    networks:
      - apk-network

  # 主应用服务
  server:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: apk-analysis-server
    restart: unless-stopped
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
    environment:
      - GIN_MODE=release
    volumes:
      - ./configs/config.yaml:/app/configs/config.yaml:ro
      - ./inbound_apks:/app/inbound_apks
      - ./results:/app/results
      - ./scripts:/app/scripts:ro
    ports:
      - "8080:8080"
    networks:
      - apk-network

networks:
  apk-network:
    driver: bridge

volumes:
  mysql_data:
  mitmproxy_certs:
EOF

    log_success "Docker Compose 配置创建完成"
}

# 更新配置文件以适配本地 Docker 网络
update_config_for_docker() {
    log_info "更新配置文件以适配 Docker 网络..."

    # 更新数据库主机为 Docker 服务名
    sed -i "s/host: .*/host: mysql/" configs/config.yaml

    # 确保 RabbitMQ 和 Redis 使用正确的主机名
    sed -i "s/host: localhost/host: redis/" configs/config.yaml

    log_success "配置文件已更新"
}

# 构建并启动服务
start_services() {
    log_info "构建并启动服务..."

    # 使用本地 compose 文件
    if docker compose version &> /dev/null; then
        COMPOSE_CMD="docker compose"
    else
        COMPOSE_CMD="docker-compose"
    fi

    # 构建镜像
    log_info "构建 Docker 镜像（可能需要几分钟）..."
    $COMPOSE_CMD -f docker-compose.local.yml build

    # 启动服务
    log_info "启动服务..."
    $COMPOSE_CMD -f docker-compose.local.yml up -d

    # 等待服务启动
    log_info "等待服务启动..."
    sleep 10

    # 检查服务状态
    $COMPOSE_CMD -f docker-compose.local.yml ps

    log_success "服务启动完成"
}

# 检查服务健康状态
check_health() {
    log_info "检查服务健康状态..."

    # 等待服务完全启动
    local max_attempts=30
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if curl -s http://localhost:8080/api/health > /dev/null 2>&1; then
            log_success "服务健康检查通过"
            return 0
        fi

        attempt=$((attempt + 1))
        log_info "等待服务启动... ($attempt/$max_attempts)"
        sleep 2
    done

    log_warn "服务可能尚未完全启动，请稍后检查"
    return 1
}

# 打印安装完成信息
print_completion() {
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                    安装完成！                              ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${BLUE}访问地址:${NC}"
    echo -e "  - Web 界面:     http://localhost:8080"
    echo -e "  - API 接口:     http://localhost:8080/api"
    echo -e "  - RabbitMQ:     http://localhost:15672 (user/pass)"
    echo ""
    echo -e "${BLUE}项目目录:${NC} $INSTALL_DIR"
    echo ""
    echo -e "${BLUE}常用命令:${NC}"
    echo -e "  - 查看日志:     docker logs -f apk-analysis-server"
    echo -e "  - 停止服务:     docker compose -f docker-compose.local.yml down"
    echo -e "  - 启动服务:     docker compose -f docker-compose.local.yml up -d"
    echo -e "  - 重启服务:     docker compose -f docker-compose.local.yml restart"
    echo ""
    echo -e "${BLUE}上传 APK 进行分析:${NC}"
    echo -e "  - 方式1: 复制 APK 到 $INSTALL_DIR/inbound_apks/ 目录"
    echo -e "  - 方式2: 通过 API 上传: curl -F 'file=@your.apk' http://localhost:8080/api/upload"
    echo ""
    echo -e "${YELLOW}注意事项:${NC}"
    echo -e "  - 动态分析需要连接 Android 设备或模拟器"
    echo -e "  - 请根据需要修改 configs/config.yaml 中的 AI API Key"
    echo -e "  - 首次运行可能需要等待数据库初始化"
    echo ""
}

# 主函数
main() {
    print_banner

    log_info "开始安装 APK 动态分析平台..."
    echo ""

    # 检查系统要求
    check_requirements

    # 克隆项目
    clone_project

    # 创建配置文件
    create_config

    # 创建 Docker Compose 覆盖文件
    create_docker_compose_override

    # 更新配置
    update_config_for_docker

    # 构建并启动服务
    start_services

    # 检查健康状态
    check_health

    # 打印完成信息
    print_completion
}

# 运行主函数
main "$@"
