#!/bin/bash
# Download frida-server for Android
# This script downloads the appropriate frida-server binary based on the device architecture

set -e

FRIDA_VERSION="16.5.9"
ARCH="x86_64"  # Genymotion 通常使用 x86_64
BIN_DIR="./bin"

echo "[*] Downloading frida-server ${FRIDA_VERSION} for ${ARCH}"

# 创建目录
mkdir -p "${BIN_DIR}"

# 下载 URL
FRIDA_URL="https://github.com/frida/frida/releases/download/${FRIDA_VERSION}/frida-server-${FRIDA_VERSION}-android-${ARCH}.xz"

echo "[+] Downloading from: ${FRIDA_URL}"

# 下载并解压
cd "${BIN_DIR}"
if [ -f "frida-server" ]; then
    echo "[!] frida-server already exists, skipping download"
    exit 0
fi

curl -L "${FRIDA_URL}" -o frida-server.xz

echo "[+] Extracting..."
xz -d frida-server.xz

echo "[+] Setting permissions..."
chmod +x frida-server

echo "[✓] frida-server downloaded successfully to ${BIN_DIR}/frida-server"
echo "[*] File size: $(du -h frida-server | cut -f1)"
