#!/bin/bash
# APK Analysis Platform - Container Entrypoint
# 同时启动 Go 应用和 Python 恶意检测服务

set -e

# 日志函数
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# 信号处理
cleanup() {
    log "Received shutdown signal, stopping services..."

    # 停止 Python 服务
    if [ -n "$MALWARE_PID" ] && kill -0 "$MALWARE_PID" 2>/dev/null; then
        log "Stopping malware detection service (PID: $MALWARE_PID)..."
        kill -TERM "$MALWARE_PID" 2>/dev/null || true
        wait "$MALWARE_PID" 2>/dev/null || true
    fi

    # 停止 Go 服务
    if [ -n "$GO_PID" ] && kill -0 "$GO_PID" 2>/dev/null; then
        log "Stopping Go server (PID: $GO_PID)..."
        kill -TERM "$GO_PID" 2>/dev/null || true
        wait "$GO_PID" 2>/dev/null || true
    fi

    log "All services stopped"
    exit 0
}

trap cleanup SIGTERM SIGINT SIGQUIT

# 环境变量默认值
MALWARE_SERVER_PORT=${MALWARE_SERVER_PORT:-5000}
MALWARE_SERVER_ENABLED=${MALWARE_SERVER_ENABLED:-true}

log "=========================================="
log "APK Analysis Platform Starting..."
log "=========================================="

# 启动 Python 恶意检测服务（后台运行）
if [ "$MALWARE_SERVER_ENABLED" = "true" ]; then
    log "Starting malware detection service on port $MALWARE_SERVER_PORT..."

    cd /app
    python3 scripts/malware_server.py &
    MALWARE_PID=$!

    # 等待 Python 服务启动
    sleep 3

    # 检查 Python 服务是否成功启动
    if kill -0 "$MALWARE_PID" 2>/dev/null; then
        log "Malware detection service started (PID: $MALWARE_PID)"

        # 等待服务就绪
        for i in {1..10}; do
            if curl -s "http://localhost:$MALWARE_SERVER_PORT/api/v1/health" > /dev/null 2>&1; then
                log "Malware detection service is ready"
                break
            fi
            sleep 1
        done
    else
        log "WARNING: Malware detection service failed to start"
    fi
else
    log "Malware detection service is disabled"
fi

# 启动 Go 应用（前台运行）
log "Starting Go application server..."
./apk-analysis-server &
GO_PID=$!

log "Go server started (PID: $GO_PID)"
log "=========================================="
log "All services started successfully"
log "  - Go API: http://localhost:8080"
if [ "$MALWARE_SERVER_ENABLED" = "true" ]; then
    log "  - Malware API: http://localhost:$MALWARE_SERVER_PORT"
fi
log "=========================================="

# 等待任一进程退出
wait -n $GO_PID $MALWARE_PID 2>/dev/null || true

# 如果任一服务退出，清理并退出
log "A service has exited, shutting down..."
cleanup
