#!/bin/bash
# 在 AOSP 模拟器上安装 mitmproxy 系统证书
# AOSP 镜像支持完整 root 和可写 /system

set -e

ADB_DEVICE=${1:-"localhost:5555"}
CERT_PATH="/tmp/mitmproxy-ca-cert.pem"

echo "=== Installing mitmproxy certificate to AOSP emulator ==="

# 1. 等待模拟器完全启动
echo "Waiting for emulator to boot..."
adb -s "$ADB_DEVICE" wait-for-device
sleep 5

# 2. 提取证书
if [ ! -f "$CERT_PATH" ]; then
    echo "Extracting certificate from mitmproxy container..."
    docker cp apk-analysis-mitmproxy:/home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem "$CERT_PATH"
fi

# 3. 计算证书 hash
CERT_HASH=$(openssl x509 -inform PEM -subject_hash_old -in "$CERT_PATH" | head -1)
echo "Certificate hash: $CERT_HASH"

# 4. 创建系统证书格式
SYSTEM_CERT="/tmp/${CERT_HASH}.0"
cp "$CERT_PATH" "$SYSTEM_CERT"

# 5. 获取 root 权限（AOSP 应该默认就有）
echo "Getting root access..."
adb -s "$ADB_DEVICE" root || echo "Already root or root not needed"
sleep 2

# 6. 重新挂载 /system 为可写
echo "Remounting /system as writable..."
adb -s "$ADB_DEVICE" shell "mount -o rw,remount /system" || \
adb -s "$ADB_DEVICE" shell "mount -o rw,remount /" || \
echo "Note: Remount may not be needed on AOSP"

# 7. 推送证书到模拟器
echo "Pushing certificate to emulator..."
adb -s "$ADB_DEVICE" push "$SYSTEM_CERT" /sdcard/${CERT_HASH}.0

# 8. 复制到系统证书目录
echo "Installing certificate to system..."
adb -s "$ADB_DEVICE" shell "cp /sdcard/${CERT_HASH}.0 /system/etc/security/cacerts/"

# 9. 设置正确的权限
echo "Setting permissions..."
adb -s "$ADB_DEVICE" shell "chmod 644 /system/etc/security/cacerts/${CERT_HASH}.0"
adb -s "$ADB_DEVICE" shell "chown root:root /system/etc/security/cacerts/${CERT_HASH}.0"

# 10. 验证安装
echo "Verifying installation..."
if adb -s "$ADB_DEVICE" shell "ls -l /system/etc/security/cacerts/${CERT_HASH}.0" 2>/dev/null; then
    echo "✓ Certificate successfully installed as system certificate!"
    echo "  Location: /system/etc/security/cacerts/${CERT_HASH}.0"
else
    echo "✗ Certificate installation failed"
    exit 1
fi

# 11. 重启系统服务以加载证书（可选）
echo "Restarting system services..."
adb -s "$ADB_DEVICE" shell "stop" || true
sleep 2
adb -s "$ADB_DEVICE" shell "start" || true

echo "=== Certificate installation complete ==="
echo "HTTPS traffic interception is now enabled!"
