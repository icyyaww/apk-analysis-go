#!/bin/bash
# 安装 mitmproxy 证书为用户证书
# 路径: /data/misc/user/0/cacerts-added/
# 优点: 不需要修改 /system，不需要重启，配合 Frida 可拦截 80-90% 应用

set -e

ADB_DEVICE=${1:-"localhost:5555"}
MITMPROXY_CONTAINER=${2:-"apk-analysis-mitmproxy-1"}
CERT_DIR="/tmp/mitmproxy_certs"
MAX_WAIT=120  # 最多等待2分钟

echo "================================================"
echo "mitmproxy 用户证书自动安装脚本"
echo "设备: $ADB_DEVICE"
echo "Mitmproxy容器: $MITMPROXY_CONTAINER"
echo "开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "================================================"

# ============================================
# 步骤 1: 从 mitmproxy 容器导出证书
# ============================================
echo -e "\n[1/6] 从 mitmproxy 容器导出证书..."
mkdir -p "$CERT_DIR"

if docker exec "$MITMPROXY_CONTAINER" test -f /home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem 2>/dev/null; then
    docker exec "$MITMPROXY_CONTAINER" cat /home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem > "$CERT_DIR/mitmproxy-ca-cert.pem"
    echo "   ✓ 证书导出成功: $CERT_DIR/mitmproxy-ca-cert.pem"
else
    echo "   ✗ 证书文件不存在，等待 mitmproxy 生成证书..."
    sleep 5
    docker exec "$MITMPROXY_CONTAINER" cat /home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem > "$CERT_DIR/mitmproxy-ca-cert.pem" || {
        echo "   ✗ 无法获取证书，请确保 mitmproxy 容器正在运行"
        exit 1
    }
fi

# ============================================
# 步骤 2: 计算证书 Hash
# ============================================
echo -e "\n[2/6] 计算证书 Hash..."
CERT_HASH=$(openssl x509 -inform PEM -subject_hash_old -in "$CERT_DIR/mitmproxy-ca-cert.pem" | head -1)
echo "   ✓ 证书 Hash: $CERT_HASH"

# ============================================
# 步骤 3: 创建 Android 格式证书文件
# ============================================
echo -e "\n[3/6] 创建 Android 格式证书..."
cat "$CERT_DIR/mitmproxy-ca-cert.pem" > "$CERT_DIR/${CERT_HASH}.0"
echo "   ✓ 证书文件: $CERT_DIR/${CERT_HASH}.0"

# ============================================
# 步骤 4: 等待设备连接
# ============================================
echo -e "\n[4/6] 等待设备连接..."
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
    if timeout 5 adb -s "$ADB_DEVICE" shell "echo ok" 2>/dev/null | grep -q "ok"; then
        echo "   ✓ 设备已连接 (${ELAPSED}秒)"
        break
    fi
    echo "   等待中... (${ELAPSED}s / ${MAX_WAIT}s)"
    sleep 5
    ELAPSED=$((ELAPSED + 5))
    adb connect "$ADB_DEVICE" 2>/dev/null || true
done

if [ $ELAPSED -ge $MAX_WAIT ]; then
    echo "   ✗ 设备连接超时"
    exit 1
fi

# ============================================
# 步骤 5: 安装用户证书
# ============================================
echo -e "\n[5/6] 安装用户证书..."

# 获取 root 权限
echo "   - 获取 root 权限..."
adb -s "$ADB_DEVICE" root 2>&1 | head -1
sleep 2

# 重新连接（root 后需要重连）
adb connect "$ADB_DEVICE" 2>/dev/null || true
sleep 1

# 创建用户证书目录（如果不存在）
echo "   - 创建用户证书目录..."
adb -s "$ADB_DEVICE" shell "mkdir -p /data/misc/user/0/cacerts-added" 2>&1

# 推送证书到临时目录
echo "   - 推送证书到设备..."
adb -s "$ADB_DEVICE" push "$CERT_DIR/${CERT_HASH}.0" /sdcard/ 2>&1 | grep -v "bytes"

# 复制到用户证书目录
echo "   - 安装到用户证书目录..."
adb -s "$ADB_DEVICE" shell "cp /sdcard/${CERT_HASH}.0 /data/misc/user/0/cacerts-added/" 2>&1

# 设置权限
echo "   - 设置证书权限..."
adb -s "$ADB_DEVICE" shell "chmod 644 /data/misc/user/0/cacerts-added/${CERT_HASH}.0" 2>&1
adb -s "$ADB_DEVICE" shell "chown system:system /data/misc/user/0/cacerts-added/${CERT_HASH}.0" 2>&1

# 清理临时文件
adb -s "$ADB_DEVICE" shell "rm -f /sdcard/${CERT_HASH}.0" 2>/dev/null || true

# ============================================
# 步骤 6: 验证安装
# ============================================
echo -e "\n[6/6] 验证证书安装..."
if adb -s "$ADB_DEVICE" shell "ls -l /data/misc/user/0/cacerts-added/${CERT_HASH}.0" 2>/dev/null | grep -q "${CERT_HASH}"; then
    echo "   ✓ 证书安装成功！"
    adb -s "$ADB_DEVICE" shell "ls -l /data/misc/user/0/cacerts-added/${CERT_HASH}.0" 2>&1 | head -1

    echo -e "\n================================================"
    echo "✅ 用户证书安装成功"
    echo "证书 Hash: $CERT_HASH"
    echo "证书路径: /data/misc/user/0/cacerts-added/${CERT_HASH}.0"
    echo "设备: $ADB_DEVICE"
    echo ""
    echo "⚠️  注意: 配合 Frida SSL Unpinning 使用可拦截 80-90% 应用"
    echo "完成时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "================================================"
    exit 0
else
    echo "   ✗ 证书安装失败"
    exit 1
fi
