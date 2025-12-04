#!/bin/bash
# 等待模拟器启动并自动安装 mitmproxy 证书

set -e

ADB_DEVICE=${1:-"localhost:5555"}
MAX_WAIT=600  # 最多等待 10 分钟
CERT_HASH="c8750f0d"

echo "=== 等待模拟器启动并安装 mitmproxy 证书 ==="
echo "开始时间: $(date '+%Y-%m-%d %H:%M:%S')"

# 1. 等待设备连接
echo "1. 等待设备连接..."
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
    if timeout 5 adb -s "$ADB_DEVICE" shell "echo ok" 2>/dev/null | grep -q "ok"; then
        echo "   ✓ 设备已连接 (${ELAPSED}秒)"
        break
    fi
    echo "   等待中... (${ELAPSED}s / ${MAX_WAIT}s)"
    sleep 10
    ELAPSED=$((ELAPSED + 10))

    # 尝试重新连接
    adb connect "$ADB_DEVICE" 2>/dev/null || true
done

if [ $ELAPSED -ge $MAX_WAIT ]; then
    echo "✗ 等待设备超时"
    exit 1
fi

# 2. 等待系统完全启动
echo -e "\n2. 等待系统完全启动..."
ELAPSED=0
while [ $ELAPSED -lt 300 ]; do
    BOOT_COMPLETED=$(adb -s "$ADB_DEVICE" shell "getprop sys.boot_completed" 2>/dev/null | tr -d '\r')
    if [ "$BOOT_COMPLETED" = "1" ]; then
        echo "   ✓ 系统启动完成 (${ELAPSED}秒)"
        break
    fi
    echo "   启动中... (${ELAPSED}s / 300s)"
    sleep 10
    ELAPSED=$((ELAPSED + 10))
done

sleep 5

# 3. 获取 root 权限
echo -e "\n3. 获取 root 权限..."
adb -s "$ADB_DEVICE" root || echo "   已经是 root"
sleep 3
adb connect "$ADB_DEVICE"
sleep 2

# 4. 检查 /system 是否可写
echo -e "\n4. 检查 /system 挂载状态..."
MOUNT_STATUS=$(adb -s "$ADB_DEVICE" shell "mount | grep ' / '" 2>/dev/null)
if echo "$MOUNT_STATUS" | grep -q "(rw"; then
    echo "   ✓ /system 已可写"
else
    echo "   /system 当前只读，尝试重新挂载..."
    adb -s "$ADB_DEVICE" shell "mount -o rw,remount /" 2>/dev/null || true
fi

# 5. 复制证书到系统目录
echo -e "\n5. 安装证书到系统..."
if adb -s "$ADB_DEVICE" shell "ls /sdcard/${CERT_HASH}.0" 2>/dev/null | grep -q "${CERT_HASH}"; then
    # 证书已在 sdcard，直接复制
    adb -s "$ADB_DEVICE" shell "cp /sdcard/${CERT_HASH}.0 /system/etc/security/cacerts/" 2>&1
    adb -s "$ADB_DEVICE" shell "chmod 644 /system/etc/security/cacerts/${CERT_HASH}.0" 2>&1
    adb -s "$ADB_DEVICE" shell "chown root:root /system/etc/security/cacerts/${CERT_HASH}.0" 2>&1
else
    echo "   证书文件不在 /sdcard，需要重新推送"
    exit 1
fi

# 6. 验证安装
echo -e "\n6. 验证证书安装..."
if adb -s "$ADB_DEVICE" shell "ls -l /system/etc/security/cacerts/${CERT_HASH}.0" 2>/dev/null | grep -q "${CERT_HASH}"; then
    echo "   ✓ 证书安装成功！"
    adb -s "$ADB_DEVICE" shell "ls -l /system/etc/security/cacerts/${CERT_HASH}.0"
    echo -e "\n=== HTTPS 流量拦截已启用 ==="
    exit 0
else
    echo "   ✗ 证书安装失败"
    exit 1
fi
