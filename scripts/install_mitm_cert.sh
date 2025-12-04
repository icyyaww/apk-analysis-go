#!/bin/bash
# 自动安装 mitmproxy 证书到 Android 模拟器
# 用于拦截 HTTPS 流量

set -e

ADB_DEVICE=${1:-"localhost:5555"}
CERT_PATH="/tmp/mitmproxy-ca-cert.pem"

echo "=== Installing mitmproxy certificate to Android emulator ==="

# 1. 提取证书（如果不存在）
if [ ! -f "$CERT_PATH" ]; then
    echo "Extracting certificate from mitmproxy container..."
    docker cp apk-analysis-mitmproxy:/home/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem "$CERT_PATH"
fi

# 2. 计算证书 hash
CERT_HASH=$(openssl x509 -inform PEM -subject_hash_old -in "$CERT_PATH" | head -1)
echo "Certificate hash: $CERT_HASH"

# 3. 创建系统证书格式
SYSTEM_CERT="/tmp/${CERT_HASH}.0"
cp "$CERT_PATH" "$SYSTEM_CERT"

# 4. 推送证书到模拟器
echo "Pushing certificate to emulator..."
adb -s "$ADB_DEVICE" push "$SYSTEM_CERT" /sdcard/

# 5. 尝试安装到系统目录（需要 root 和可写 /system）
echo "Attempting to install certificate to system..."
adb -s "$ADB_DEVICE" root 2>/dev/null || echo "Warning: Could not get root access"
sleep 2

# 尝试重新挂载 /system 为可写
adb -s "$ADB_DEVICE" shell "mount -o rw,remount /system 2>/dev/null" || \
adb -s "$ADB_DEVICE" shell "mount -o rw,remount / 2>/dev/null" || \
echo "Warning: Could not remount /system as writable"

# 尝试复制到系统目录（可能失败，但不影响用户证书安装）
adb -s "$ADB_DEVICE" shell "cp /sdcard/${CERT_HASH}.0 /system/etc/security/cacerts/ 2>/dev/null && \
    chmod 644 /system/etc/security/cacerts/${CERT_HASH}.0" || \
echo "Note: System certificate installation failed (expected on Android 10+)"

# 6. 安装为用户证书（作为备选方案）
echo "Installing as user certificate..."
adb -s "$ADB_DEVICE" push "$CERT_PATH" /sdcard/Download/mitmproxy-ca-cert.pem

# 7. 验证证书是否已安装
echo "Verifying installation..."
if adb -s "$ADB_DEVICE" shell "ls /system/etc/security/cacerts/${CERT_HASH}.0" 2>/dev/null; then
    echo "✓ Certificate installed as system certificate"
else
    echo "⚠ Certificate not installed as system certificate (Android 10+ limitation)"
    echo "  User certificate is available at: /sdcard/Download/mitmproxy-ca-cert.pem"
    echo "  Manual installation: Settings > Security > Install from storage"
fi

echo "=== Certificate installation complete ==="
