#!/bin/bash
# APK Analysis Platform - Production Deployment Script
# APK 动态分析平台 - 生产环境部署脚本
#
# 用途: 自动化部署生产环境
# 使用: ./deploy.sh [options]
#
# 选项:
#   --skip-build    跳过 Docker 镜像构建
#   --skip-backup   跳过数据库备份
#   --force         强制部署,忽略确认
#   --rollback      回滚到上一个版本
#   --help          显示帮助信息

set -e  # 遇到错误立即退出
set -u  # 使用未定义变量时报错

# ============================================
# 颜色输出
# ============================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ============================================
# 日志函数
# ============================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# ============================================
# 配置变量
# ============================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="$PROJECT_ROOT/docker-compose.prod.yml"
ENV_FILE="$PROJECT_ROOT/.env"
BACKUP_DIR="$PROJECT_ROOT/backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 默认选项
SKIP_BUILD=false
SKIP_BACKUP=false
FORCE_DEPLOY=false
ROLLBACK=false

# ============================================
# 解析命令行参数
# ============================================

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --skip-backup)
            SKIP_BACKUP=true
            shift
            ;;
        --force)
            FORCE_DEPLOY=true
            shift
            ;;
        --rollback)
            ROLLBACK=true
            shift
            ;;
        --help)
            echo "APK Analysis Platform - Production Deployment Script"
            echo ""
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --skip-build    Skip Docker image building"
            echo "  --skip-backup   Skip database backup"
            echo "  --force         Force deployment without confirmation"
            echo "  --rollback      Rollback to previous version"
            echo "  --help          Show this help message"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# ============================================
# 前置检查
# ============================================

log_info "=== 前置检查 ==="

# 检查 Docker
if ! command -v docker &> /dev/null; then
    log_error "Docker 未安装,请先安装 Docker"
    exit 1
fi
log_success "Docker 已安装: $(docker --version)"

# 检查 Docker Compose
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    log_error "Docker Compose 未安装,请先安装 Docker Compose"
    exit 1
fi

if docker compose version &> /dev/null; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi
log_success "Docker Compose 已安装: $($DOCKER_COMPOSE version)"

# 检查配置文件
if [ ! -f "$COMPOSE_FILE" ]; then
    log_error "Docker Compose 配置文件不存在: $COMPOSE_FILE"
    exit 1
fi
log_success "Docker Compose 配置文件存在"

if [ ! -f "$ENV_FILE" ]; then
    log_error ".env 文件不存在,请从 .env.example 创建"
    log_info "运行: cp .env.example .env"
    exit 1
fi
log_success ".env 配置文件存在"

# 检查必要目录
REQUIRED_DIRS=(
    "$PROJECT_ROOT/results"
    "$PROJECT_ROOT/logs"
    "$PROJECT_ROOT/inbound_apks"
    "$PROJECT_ROOT/configs"
)

for dir in "${REQUIRED_DIRS[@]}"; do
    if [ ! -d "$dir" ]; then
        log_warning "目录不存在,正在创建: $dir"
        mkdir -p "$dir"
    fi
done
log_success "所有必要目录已就绪"

# ============================================
# 回滚功能
# ============================================

if [ "$ROLLBACK" = true ]; then
    log_info "=== 开始回滚 ==="

    # 检查是否有备份
    if [ ! -d "$BACKUP_DIR" ] || [ -z "$(ls -A $BACKUP_DIR)" ]; then
        log_error "没有找到备份文件"
        exit 1
    fi

    # 列出可用备份
    log_info "可用的备份:"
    ls -lh "$BACKUP_DIR"

    # 停止当前服务
    log_info "停止当前服务..."
    cd "$PROJECT_ROOT"
    $DOCKER_COMPOSE -f "$COMPOSE_FILE" down

    # 恢复最新备份 (这里需要根据实际备份策略实现)
    log_warning "请手动恢复数据库备份"
    log_info "备份位置: $BACKUP_DIR"

    # 重新启动服务
    log_info "重新启动服务..."
    $DOCKER_COMPOSE -f "$COMPOSE_FILE" up -d

    log_success "回滚完成"
    exit 0
fi

# ============================================
# 用户确认
# ============================================

if [ "$FORCE_DEPLOY" = false ]; then
    echo ""
    log_warning "即将部署到生产环境"
    log_info "Docker Compose 文件: $COMPOSE_FILE"
    log_info "环境变量文件: $ENV_FILE"
    echo ""
    read -p "确认继续部署? (yes/no): " -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        log_info "部署已取消"
        exit 0
    fi
fi

# ============================================
# 数据库备份
# ============================================

if [ "$SKIP_BACKUP" = false ]; then
    log_info "=== 数据库备份 ==="

    mkdir -p "$BACKUP_DIR"

    # 检查数据库容器是否运行
    if docker ps --format '{{.Names}}' | grep -q "apk-analysis-mysql"; then
        log_info "正在备份 MySQL 数据库..."

        # 从 .env 读取数据库配置
        source "$ENV_FILE"

        BACKUP_FILE="$BACKUP_DIR/mysql_backup_${TIMESTAMP}.sql.gz"

        docker exec apk-analysis-mysql sh -c \
            "mysqldump -u${MYSQL_USER} -p${MYSQL_PASS} ${MYSQL_DB} | gzip" \
            > "$BACKUP_FILE"

        if [ $? -eq 0 ]; then
            log_success "数据库备份完成: $BACKUP_FILE"
            log_info "备份大小: $(du -h $BACKUP_FILE | cut -f1)"
        else
            log_error "数据库备份失败"
            exit 1
        fi
    else
        log_warning "数据库容器未运行,跳过备份"
    fi
fi

# ============================================
# 构建 Docker 镜像
# ============================================

if [ "$SKIP_BUILD" = false ]; then
    log_info "=== 构建 Docker 镜像 ==="

    cd "$PROJECT_ROOT"

    log_info "正在构建 apk-analysis-go:latest..."
    docker build -t apk-analysis-go:latest .

    if [ $? -eq 0 ]; then
        log_success "Docker 镜像构建成功"

        # 显示镜像信息
        IMAGE_SIZE=$(docker images apk-analysis-go:latest --format "{{.Size}}")
        log_info "镜像大小: $IMAGE_SIZE"
    else
        log_error "Docker 镜像构建失败"
        exit 1
    fi
fi

# ============================================
# 停止旧服务
# ============================================

log_info "=== 停止旧服务 ==="

cd "$PROJECT_ROOT"

if $DOCKER_COMPOSE -f "$COMPOSE_FILE" ps | grep -q "Up"; then
    log_info "正在停止旧服务..."
    $DOCKER_COMPOSE -f "$COMPOSE_FILE" down
    log_success "旧服务已停止"
else
    log_info "没有运行中的服务"
fi

# ============================================
# 启动新服务
# ============================================

log_info "=== 启动新服务 ==="

cd "$PROJECT_ROOT"

log_info "正在启动服务..."
$DOCKER_COMPOSE -f "$COMPOSE_FILE" up -d

if [ $? -eq 0 ]; then
    log_success "服务启动成功"
else
    log_error "服务启动失败"
    exit 1
fi

# 等待服务启动
log_info "等待服务启动完成..."
sleep 10

# ============================================
# 健康检查
# ============================================

log_info "=== 健康检查 ==="

# 检查容器状态
log_info "检查容器状态..."
$DOCKER_COMPOSE -f "$COMPOSE_FILE" ps

# 检查主应用健康状态
MAX_RETRIES=30
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if curl -f http://localhost:8080/api/health > /dev/null 2>&1; then
        log_success "主应用健康检查通过"
        break
    fi

    RETRY_COUNT=$((RETRY_COUNT + 1))
    log_info "等待主应用启动... ($RETRY_COUNT/$MAX_RETRIES)"
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    log_error "主应用健康检查失败"
    log_info "查看日志: $DOCKER_COMPOSE -f $COMPOSE_FILE logs apk-analysis-server"
    exit 1
fi

# ============================================
# 显示服务状态
# ============================================

log_info "=== 服务状态 ==="

echo ""
log_info "服务访问地址:"
echo "  - API 服务:        http://localhost:8080"
echo "  - Prometheus:      http://localhost:9091"
echo "  - Grafana:         http://localhost:3000"
echo "  - RabbitMQ 管理:   http://localhost:15672"
echo ""

log_info "常用命令:"
echo "  - 查看日志:        $DOCKER_COMPOSE -f $COMPOSE_FILE logs -f"
echo "  - 查看服务状态:    $DOCKER_COMPOSE -f $COMPOSE_FILE ps"
echo "  - 停止服务:        $DOCKER_COMPOSE -f $COMPOSE_FILE down"
echo "  - 重启服务:        $DOCKER_COMPOSE -f $COMPOSE_FILE restart"
echo ""

# ============================================
# 清理旧备份
# ============================================

if [ -d "$BACKUP_DIR" ]; then
    log_info "清理超过 30 天的旧备份..."
    find "$BACKUP_DIR" -name "mysql_backup_*.sql.gz" -mtime +30 -delete
    log_success "旧备份清理完成"
fi

# ============================================
# 部署完成
# ============================================

log_success "=== 部署完成 ==="
log_info "部署时间: $(date)"
log_info "下一步: 运行 ./verify.sh 进行部署验证"
