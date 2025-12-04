#!/bin/bash
# 一次性初始化模拟器以支持系统证书安装
# 功能：禁用 dm-verity，启用 overlayfs，重启模拟器

set -e

ADB_DEVICE=${1:-"localhost:5555"}
MAX_WAIT=120

echo "================================================"
echo "Android 模拟器 MITM 初始化脚本"
echo "设备: $ADB_DEVICE"
echo "开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "================================================"

# ============================================
# 步骤 1: 等待设备连接
# ============================================
echo -e "\n[1/5] 等待设备连接..."
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
# 步骤 2: 获取 root 权限
# ============================================
echo -e "\n[2/5] 获取 root 权限..."
adb -s "$ADB_DEVICE" root 2>&1 | head -1
sleep 2

# 重新连接
adb connect "$ADB_DEVICE" 2>/dev/null || true
sleep 1

# ============================================
# 步骤 3: 禁用 verity 并启用 overlayfs
# ============================================
echo -e "\n[3/5] 禁用 verity 并启用 overlayfs..."
adb -s "$ADB_DEVICE" remount 2>&1

# ============================================
# 步骤 4: 重启设备
# ============================================
echo -e "\n[4/5] 重启设备以应用更改..."
adb -s "$ADB_DEVICE" reboot 2>&1
echo "   ✓ 重启命令已发送"

# 等待设备断开
sleep 5

# ============================================
# 步骤 5: 等待设备重新启动
# ============================================
echo -e "\n[5/5] 等待设备重新启动..."
ELAPSED=0
MAX_BOOT_WAIT=180  # 最多等待 3 分钟

while [ $ELAPSED -lt $MAX_BOOT_WAIT ]; do
    if timeout 5 adb -s "$ADB_DEVICE" shell "getprop sys.boot_completed" 2>/dev/null | grep -q "1"; then
        echo "   ✓ 设备启动完成 (${ELAPSED}秒)"
        break
    fi
    echo "   启动中... (${ELAPSED}s / ${MAX_BOOT_WAIT}s)"
    sleep 10
    ELAPSED=$((ELAPSED + 10))
    adb connect "$ADB_DEVICE" 2>/dev/null || true
done

if [ $ELAPSED -ge $MAX_BOOT_WAIT ]; then
    echo "   ✗ 设备启动超时"
    exit 1
fi

# 等待额外 5 秒确保系统完全就绪
sleep 5

# 验证 overlayfs 是否生效
echo -e "\n验证 overlayfs 状态..."
adb -s "$ADB_DEVICE" root 2>&1 | head -1
sleep 1
MOUNT_INFO=$(adb -s "$ADB_DEVICE" shell "mount | grep ' /system '" 2>/dev/null)
if echo "$MOUNT_INFO" | grep -q "overlay"; then
    echo "   ✓ overlayfs 已启用"
else
    echo "   ⚠  overlayfs 可能未启用，但可以尝试继续"
fi

echo -e "\n================================================"
echo "✅ 模拟器初始化完成！"
echo "设备: $ADB_DEVICE"
echo "现在可以安装系统证书了"
echo "完成时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "================================================"
