#!/bin/bash
# 完整的证书准备和安装脚本
# 功能：从 mitmproxy 导出证书 -> 推送到模拟器 -> 安装到系统

set -e

ADB_DEVICE=${1:-"localhost:5555"}
MITMPROXY_CONTAINER=${2:-"apk-analysis-mitmproxy-1"}
CERT_DIR="/tmp/mitmproxy_certs"
MAX_WAIT=120  # 最多等待2分钟

echo "================================================"
echo "mitmproxy 证书自动安装脚本"
echo "设备: $ADB_DEVICE"
echo "Mitmproxy容器: $MITMPROXY_CONTAINER"
echo "开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "================================================"

# ============================================
# 步骤 1: 从 mitmproxy 容器导出证书
# ============================================
echo -e "\n[1/7] 从 mitmproxy 容器导出证书..."
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
echo -e "\n[2/7] 计算证书 Hash..."
CERT_HASH=$(openssl x509 -inform PEM -subject_hash_old -in "$CERT_DIR/mitmproxy-ca-cert.pem" | head -1)
echo "   ✓ 证书 Hash: $CERT_HASH"

# ============================================
# 步骤 3: 创建 Android 格式证书文件
# ============================================
echo -e "\n[3/7] 创建 Android 格式证书..."
cat "$CERT_DIR/mitmproxy-ca-cert.pem" > "$CERT_DIR/${CERT_HASH}.0"
echo "   ✓ 证书文件: $CERT_DIR/${CERT_HASH}.0"

# ============================================
# 步骤 4: 等待设备连接
# ============================================
echo -e "\n[4/7] 等待设备连接..."
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
# 步骤 5: 推送证书到模拟器
# ============================================
echo -e "\n[5/7] 推送证书到设备..."
adb -s "$ADB_DEVICE" push "$CERT_DIR/${CERT_HASH}.0" /sdcard/ 2>&1 | grep -v "bytes"
echo "   ✓ 证书已推送到 /sdcard/${CERT_HASH}.0"

# ============================================
# 步骤 6: 安装证书到系统目录
# ============================================
echo -e "\n[6/7] 安装证书到系统..."

# 获取 root 权限
echo "   - 获取 root 权限..."
adb -s "$ADB_DEVICE" root 2>&1 | head -1
sleep 2

# 重新连接（root 后需要重连）
adb connect "$ADB_DEVICE" 2>/dev/null || true
sleep 1

# 检查证书是否已安装
if adb -s "$ADB_DEVICE" shell "ls /system/etc/security/cacerts/${CERT_HASH}.0" 2>/dev/null | grep -q "${CERT_HASH}"; then
    echo "   ✓ 证书已存在，跳过安装"
else
    # 使用 adb remount（支持 overlayfs）
    echo "   - 使用 adb remount 重新挂载系统..."
    adb -s "$ADB_DEVICE" remount 2>&1 | head -3

    # 复制证书
    echo "   - 复制证书到系统目录..."
    adb -s "$ADB_DEVICE" shell "cp /sdcard/${CERT_HASH}.0 /system/etc/security/cacerts/" 2>&1

    # 设置权限
    echo "   - 设置证书权限..."
    adb -s "$ADB_DEVICE" shell "chmod 644 /system/etc/security/cacerts/${CERT_HASH}.0" 2>&1
    adb -s "$ADB_DEVICE" shell "chown root:root /system/etc/security/cacerts/${CERT_HASH}.0" 2>&1
fi

# ============================================
# 步骤 7: 验证安装
# ============================================
echo -e "\n[7/7] 验证证书安装..."
if adb -s "$ADB_DEVICE" shell "ls -l /system/etc/security/cacerts/${CERT_HASH}.0" 2>/dev/null | grep -q "${CERT_HASH}"; then
    echo "   ✓ 证书安装成功！"
    adb -s "$ADB_DEVICE" shell "ls -l /system/etc/security/cacerts/${CERT_HASH}.0" 2>&1 | head -1

    # 清理临时文件
    adb -s "$ADB_DEVICE" shell "rm -f /sdcard/${CERT_HASH}.0" 2>/dev/null || true

    echo -e "\n================================================"
    echo "✅ HTTPS 流量拦截已启用"
    echo "证书 Hash: $CERT_HASH"
    echo "设备: $ADB_DEVICE"
    echo "完成时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "================================================"
    exit 0
else
    echo "   ✗ 证书安装失败"
    exit 1
fi
