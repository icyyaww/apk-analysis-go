#!/bin/bash
# APK Analysis Platform - Production Verification Script
# APK 动态分析平台 - 生产环境验证脚本
#
# 用途: 验证生产环境部署是否正常
# 使用: ./verify.sh

set -e

# ============================================
# 颜色输出
# ============================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# ============================================
# 日志函数
# ============================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

# ============================================
# 测试结果统计
# ============================================

TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

test_pass() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    PASSED_TESTS=$((PASSED_TESTS + 1))
    log_success "$1"
}

test_fail() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    FAILED_TESTS=$((FAILED_TESTS + 1))
    log_error "$1"
}

# ============================================
# 配置变量
# ============================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
API_URL="http://localhost:8080"
PROMETHEUS_URL="http://localhost:9091"
GRAFANA_URL="http://localhost:3000"
RABBITMQ_URL="http://localhost:15672"

# ============================================
# 验证开始
# ============================================

echo ""
log_info "=========================================="
log_info "  APK Analysis Platform"
log_info "  生产环境验证脚本"
log_info "=========================================="
echo ""

# ============================================
# 1. Docker 容器状态检查
# ============================================

log_info "【1/8】检查 Docker 容器状态..."
echo ""

EXPECTED_CONTAINERS=(
    "apk-analysis-server"
    "apk-analysis-mysql"
    "apk-analysis-rabbitmq"
    "apk-analysis-redis"
    "apk-analysis-prometheus"
    "apk-analysis-grafana"
)

for container in "${EXPECTED_CONTAINERS[@]}"; do
    if docker ps --format '{{.Names}}' | grep -q "$container"; then
        STATUS=$(docker inspect --format='{{.State.Status}}' "$container")
        if [ "$STATUS" == "running" ]; then
            test_pass "容器 $container 运行正常"
        else
            test_fail "容器 $container 状态异常: $STATUS"
        fi
    else
        test_fail "容器 $container 不存在或未运行"
    fi
done

echo ""

# ============================================
# 2. 端口监听检查
# ============================================

log_info "【2/8】检查端口监听状态..."
echo ""

EXPECTED_PORTS=(
    "8080:API 服务"
    "9090:Metrics"
    "3306:MySQL"
    "5672:RabbitMQ AMQP"
    "15672:RabbitMQ 管理"
    "6379:Redis"
    "9091:Prometheus"
    "3000:Grafana"
)

for port_desc in "${EXPECTED_PORTS[@]}"; do
    IFS=':' read -r port desc <<< "$port_desc"
    if netstat -tuln 2>/dev/null | grep -q ":$port " || ss -tuln 2>/dev/null | grep -q ":$port "; then
        test_pass "端口 $port ($desc) 监听正常"
    else
        test_fail "端口 $port ($desc) 未监听"
    fi
done

echo ""

# ============================================
# 3. 健康检查端点
# ============================================

log_info "【3/8】检查健康检查端点..."
echo ""

# 主应用健康检查
if curl -f -s "$API_URL/api/health" > /dev/null; then
    HEALTH_RESPONSE=$(curl -s "$API_URL/api/health")
    test_pass "API 健康检查通过: $HEALTH_RESPONSE"
else
    test_fail "API 健康检查失败"
fi

# Prometheus 健康检查
if curl -f -s "$PROMETHEUS_URL/-/healthy" > /dev/null; then
    test_pass "Prometheus 健康检查通过"
else
    test_fail "Prometheus 健康检查失败"
fi

# Grafana 健康检查
if curl -f -s "$GRAFANA_URL/api/health" > /dev/null; then
    test_pass "Grafana 健康检查通过"
else
    test_fail "Grafana 健康检查失败"
fi

echo ""

# ============================================
# 4. API 端点测试
# ============================================

log_info "【4/8】测试 API 端点..."
echo ""

# 获取任务列表
if curl -f -s "$API_URL/api/tasks" > /dev/null; then
    test_pass "GET /api/tasks - 获取任务列表"
else
    test_fail "GET /api/tasks - 获取任务列表失败"
fi

# 获取系统统计
if curl -f -s "$API_URL/api/stats" > /dev/null; then
    STATS=$(curl -s "$API_URL/api/stats")
    test_pass "GET /api/stats - 获取系统统计: $STATS"
else
    test_fail "GET /api/stats - 获取系统统计失败"
fi

echo ""

# ============================================
# 5. 数据库连接测试
# ============================================

log_info "【5/8】测试数据库连接..."
echo ""

# 检查 MySQL 容器
if docker exec apk-analysis-mysql mysqladmin ping -h localhost -uroot -p${MYSQL_ROOT_PASSWORD:-root} 2>/dev/null | grep -q "mysqld is alive"; then
    test_pass "MySQL 数据库连接正常"

    # 检查表是否存在
    TABLES=$(docker exec apk-analysis-mysql mysql -uroot -p${MYSQL_ROOT_PASSWORD:-root} -D ${MYSQL_DB:-apk_analysis} -e "SHOW TABLES;" 2>/dev/null | tail -n +2)
    if [ -n "$TABLES" ]; then
        TABLE_COUNT=$(echo "$TABLES" | wc -l)
        test_pass "数据库表已创建 ($TABLE_COUNT 张表)"
    else
        test_fail "数据库表未创建"
    fi
else
    test_fail "MySQL 数据库连接失败"
fi

echo ""

# ============================================
# 6. RabbitMQ 连接测试
# ============================================

log_info "【6/8】测试 RabbitMQ 连接..."
echo ""

# 检查 RabbitMQ 队列
if docker exec apk-analysis-rabbitmq rabbitmqctl list_queues 2>/dev/null | grep -q "apk_tasks"; then
    test_pass "RabbitMQ 队列已创建"
else
    test_warning "RabbitMQ 队列未创建 (首次运行时正常)"
fi

# 检查 RabbitMQ 连接数
CONNECTIONS=$(docker exec apk-analysis-rabbitmq rabbitmqctl list_connections 2>/dev/null | wc -l)
if [ $CONNECTIONS -gt 1 ]; then
    test_pass "RabbitMQ 有活跃连接 ($((CONNECTIONS - 1)) 个)"
else
    test_warning "RabbitMQ 暂无活跃连接 (应用可能未连接)"
fi

echo ""

# ============================================
# 7. Redis 连接测试
# ============================================

log_info "【7/8】测试 Redis 连接..."
echo ""

# Redis PING 测试
if docker exec apk-analysis-redis redis-cli ping 2>/dev/null | grep -q "PONG"; then
    test_pass "Redis 连接正常"

    # 检查 Redis 内存使用
    REDIS_MEM=$(docker exec apk-analysis-redis redis-cli info memory 2>/dev/null | grep "used_memory_human" | cut -d: -f2 | tr -d '\r')
    test_pass "Redis 内存使用: $REDIS_MEM"
else
    test_fail "Redis 连接失败"
fi

echo ""

# ============================================
# 8. 文件系统检查
# ============================================

log_info "【8/8】检查文件系统..."
echo ""

REQUIRED_DIRS=(
    "$PROJECT_ROOT/results"
    "$PROJECT_ROOT/logs"
    "$PROJECT_ROOT/inbound_apks"
    "$PROJECT_ROOT/configs"
)

for dir in "${REQUIRED_DIRS[@]}"; do
    if [ -d "$dir" ] && [ -w "$dir" ]; then
        test_pass "目录存在且可写: $dir"
    else
        test_fail "目录不存在或不可写: $dir"
    fi
done

echo ""

# ============================================
# 性能指标检查
# ============================================

log_info "【性能指标】"
echo ""

# 容器资源使用情况
log_info "容器资源使用:"
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" \
    apk-analysis-server apk-analysis-mysql apk-analysis-rabbitmq apk-analysis-redis 2>/dev/null || true

echo ""

# 磁盘使用情况
log_info "磁盘使用情况:"
df -h "$PROJECT_ROOT" | tail -n 1

echo ""

# ============================================
# 测试结果汇总
# ============================================

echo ""
log_info "=========================================="
log_info "  验证结果汇总"
log_info "=========================================="
echo ""

log_info "总测试数: $TOTAL_TESTS"
log_success "通过: $PASSED_TESTS"

if [ $FAILED_TESTS -gt 0 ]; then
    log_error "失败: $FAILED_TESTS"
    echo ""
    log_error "部署验证失败! 请检查上述错误信息"
    echo ""
    log_info "常用排查命令:"
    echo "  - 查看所有容器日志:     docker-compose -f docker-compose.prod.yml logs"
    echo "  - 查看主应用日志:       docker logs apk-analysis-server"
    echo "  - 查看 MySQL 日志:      docker logs apk-analysis-mysql"
    echo "  - 重启所有服务:         docker-compose -f docker-compose.prod.yml restart"
    echo ""
    exit 1
else
    echo ""
    log_success "========================================"
    log_success "  所有验证通过! 系统运行正常"
    log_success "========================================"
    echo ""

    log_info "服务访问地址:"
    echo "  - API 服务:        $API_URL"
    echo "  - API 文档:        $API_URL/swagger/index.html"
    echo "  - Prometheus:      $PROMETHEUS_URL"
    echo "  - Grafana:         $GRAFANA_URL"
    echo "  - RabbitMQ 管理:   $RABBITMQ_URL"
    echo ""

    log_info "下一步:"
    echo "  1. 访问 Grafana 配置监控面板"
    echo "  2. 上传测试 APK 文件到 inbound_apks/ 目录"
    echo "  3. 监控任务执行情况"
    echo ""

    exit 0
fi
