#!/bin/bash

# APK Analysis Platform - Go 安装脚本
# 用途: 自动安装 Go 1.21.5
# 使用: bash scripts/install-go.sh

set -e

GO_VERSION="1.21.5"
GO_ARCH="linux-amd64"
GO_TARBALL="go${GO_VERSION}.${GO_ARCH}.tar.gz"
GO_URL="https://go.dev/dl/${GO_TARBALL}"

echo "========================================="
echo "  APK Analysis Platform - Go 安装"
echo "========================================="
echo ""
echo "Go 版本: ${GO_VERSION}"
echo "架构: ${GO_ARCH}"
echo ""

# 检查是否已安装 Go
if command -v go &> /dev/null; then
    CURRENT_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    echo "✓ 检测到已安装 Go ${CURRENT_VERSION}"

    if [ "$CURRENT_VERSION" == "$GO_VERSION" ]; then
        echo "✓ Go 版本正确,无需重新安装"
        exit 0
    else
        echo "⚠ Go 版本不匹配,将升级到 ${GO_VERSION}"
    fi
fi

# 下载 Go
echo ""
echo "步骤 1/4: 下载 Go ${GO_VERSION}..."
cd /tmp
if [ -f "${GO_TARBALL}" ]; then
    echo "✓ 发现已下载的文件,跳过下载"
else
    wget ${GO_URL}
    echo "✓ 下载完成"
fi

# 安装 Go
echo ""
echo "步骤 2/4: 安装 Go 到 /usr/local/go..."
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf ${GO_TARBALL}
echo "✓ 安装完成"

# 配置环境变量
echo ""
echo "步骤 3/4: 配置环境变量..."

# 检查是否已配置
if grep -q "/usr/local/go/bin" ~/.bashrc; then
    echo "✓ 环境变量已配置,跳过"
else
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    echo 'export GOPATH=$HOME/go' >> ~/.bashrc
    echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
    echo "✓ 环境变量已添加到 ~/.bashrc"
fi

# 使配置生效
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# 验证安装
echo ""
echo "步骤 4/4: 验证安装..."
if /usr/local/go/bin/go version &> /dev/null; then
    echo "✓ Go 安装成功!"
    /usr/local/go/bin/go version
else
    echo "✗ Go 安装失败"
    exit 1
fi

# 清理临时文件
echo ""
echo "清理临时文件..."
rm -f /tmp/${GO_TARBALL}
echo "✓ 清理完成"

# 提示
echo ""
echo "========================================="
echo "  安装成功!"
echo "========================================="
echo ""
echo "请运行以下命令使环境变量生效:"
echo "  source ~/.bashrc"
echo ""
echo "或者重新登录 shell"
echo ""
echo "验证安装:"
echo "  go version"
echo ""
echo "下一步:"
echo "  cd /home/icyyaww/project/动态apk解析/apk-analysis-go"
echo "  make deps    # 下载依赖"
echo "  make build   # 构建项目"
echo "  make run     # 运行项目"
echo ""
