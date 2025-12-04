#!/bin/bash
# Docker 容器启动后自动安装证书
# 用途：当 apk-analysis-server 容器启动后，自动为所有模拟器安装 mitmproxy 用户证书
# 调用时机：docker-compose.prod.yml 中的 command 或 手动执行

set -e

echo "========================================"
echo "证书自动安装脚本"
echo "开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================"

# 等待 mitmproxy 容器启动并生成证书（最多等待60秒）
echo -e "\n[1/3] 等待 mitmproxy 容器启动..."
MAX_WAIT=60
ELAPSED=0

while [ $ELAPSED -lt $MAX_WAIT ]; do
    if docker exec apk-analysis-mitmproxy-1 test -f /home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem 2>/dev/null; then
        echo "   ✓ mitmproxy-1 证书已生成"
        break
    fi
    echo "   等待 mitmproxy-1 生成证书... (${ELAPSED}s / ${MAX_WAIT}s)"
    sleep 5
    ELAPSED=$((ELAPSED + 5))
done

# 等待模拟器启动（最多等待180秒）
echo -e "\n[2/3] 等待模拟器启动..."
MAX_BOOT_WAIT=180

# 等待 emulator-1
ELAPSED=0
echo "   - 等待 emulator-1..."
while [ $ELAPSED -lt $MAX_BOOT_WAIT ]; do
    BOOT_STATUS=$(docker exec apk-analysis-android-emulator-1 adb shell "getprop sys.boot_completed" 2>/dev/null | tr -d '\r' || echo "0")
    if [ "$BOOT_STATUS" = "1" ]; then
        echo "   ✓ emulator-1 启动完成 (${ELAPSED}秒)"
        break
    fi
    sleep 10
    ELAPSED=$((ELAPSED + 10))
done

# 等待 emulator-2
ELAPSED=0
echo "   - 等待 emulator-2..."
while [ $ELAPSED -lt $MAX_BOOT_WAIT ]; do
    BOOT_STATUS=$(docker exec apk-analysis-android-emulator-2 adb shell "getprop sys.boot_completed" 2>/dev/null | tr -d '\r' || echo "0")
    if [ "$BOOT_STATUS" = "1" ]; then
        echo "   ✓ emulator-2 启动完成 (${ELAPSED}秒)"
        break
    fi
    sleep 10
    ELAPSED=$((ELAPSED + 10))
done

# 安装证书
echo -e "\n[3/3] 为所有模拟器安装证书..."

# 安装证书到 emulator-1
echo -e "\n--- 安装证书到 emulator-1 ---"
bash /app/scripts/install_user_cert.sh android-emulator-1:5555 apk-analysis-mitmproxy-1 || {
    echo "   ⚠ emulator-1 证书安装失败，跳过"
}

# 安装证书到 emulator-2
echo -e "\n--- 安装证书到 emulator-2 ---"
bash /app/scripts/install_user_cert.sh android-emulator-2:5555 apk-analysis-mitmproxy-2 || {
    echo "   ⚠ emulator-2 证书安装失败，跳过"
}

echo -e "\n========================================"
echo "✅ 证书安装脚本执行完成"
echo "完成时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================"
