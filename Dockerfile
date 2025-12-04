# APK Analysis Platform - Production Dockerfile
# Multi-stage build for optimized image size

# ============================================
# Stage 1: Builder
# ============================================
FROM golang:1.23-alpine AS builder

# 安装构建依赖
RUN apk add --no-cache git make gcc musl-dev

# 设置 Go 代理 (使用国内镜像加速)
ENV GOPROXY=https://goproxy.cn,direct
ENV GO111MODULE=on

# 设置工作目录
WORKDIR /build

# 先复制依赖文件（go.mod 和 go.sum）
COPY go.mod go.sum ./

# 下载依赖（这层会被缓存，只有当 go.mod 或 go.sum 变化时才重新下载）
RUN go mod download

# 再复制所有源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o apk-analysis-server \
    ./cmd/server

# ============================================
# Stage 2: Runtime (使用 Debian 以支持 Androguard 4.x)
# ============================================
FROM debian:bookworm-slim

# 安装运行时依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    wget \
    curl \
    unzip \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# 安装 Androguard 4.x (深度 DEX 分析，提取代码中硬编码的 URL)
# 版本 4.1.3 性能大幅提升：63MB APK 从 16+ 分钟降至 2-3 分钟 (5-8x 性能提升)
# 使用国内 PyPI 镜像加速安装
RUN pip3 install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple \
    "androguard>=4.1.0,<5.0.0" --break-system-packages

# 安装 Android SDK Platform Tools (adb) 和 Build Tools (aapt2)
ENV ANDROID_SDK_ROOT=/opt/android-sdk
ENV BUILD_TOOLS_VERSION=34.0.0

# 安装 Platform Tools (adb)
RUN mkdir -p ${ANDROID_SDK_ROOT}/platform-tools && \
    cd /tmp && \
    wget -q https://dl.google.com/android/repository/platform-tools-latest-linux.zip && \
    unzip -q platform-tools-latest-linux.zip && \
    mv platform-tools/* ${ANDROID_SDK_ROOT}/platform-tools/ && \
    rm -rf platform-tools platform-tools-latest-linux.zip

# 安装 Build Tools (aapt2)
RUN mkdir -p ${ANDROID_SDK_ROOT}/build-tools/${BUILD_TOOLS_VERSION} && \
    cd /tmp && \
    wget -q https://dl.google.com/android/repository/build-tools_r${BUILD_TOOLS_VERSION}-linux.zip && \
    unzip -q build-tools_r${BUILD_TOOLS_VERSION}-linux.zip -d ${ANDROID_SDK_ROOT}/build-tools/${BUILD_TOOLS_VERSION} && \
    rm -f build-tools_r${BUILD_TOOLS_VERSION}-linux.zip && \
    chmod +x ${ANDROID_SDK_ROOT}/build-tools/${BUILD_TOOLS_VERSION}/android-${BUILD_TOOLS_VERSION}/* || true

ENV PATH="${ANDROID_SDK_ROOT}/platform-tools:${ANDROID_SDK_ROOT}/build-tools/${BUILD_TOOLS_VERSION}:${PATH}"

# 创建非 root 用户
RUN groupadd -g 1000 appuser && \
    useradd -u 1000 -g appuser -m -s /bin/bash appuser

# 设置工作目录
WORKDIR /app

# 从 builder 复制编译好的二进制文件
COPY --from=builder /build/apk-analysis-server .

# 复制配置文件 (如果存在)
COPY --from=builder /build/configs ./configs

# 复制 web 前端资源
COPY --from=builder /build/web ./web

# 复制 scripts 目录 (包含 LIEF 提取脚本)
COPY --from=builder /build/scripts ./scripts

# 创建必要目录
RUN mkdir -p \
    /app/results \
    /app/logs \
    /app/inbound_apks \
    /app/backups && \
    chown -R appuser:appuser /app

# 切换到非 root 用户
USER appuser

# 暴露端口
EXPOSE 8080 9090

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/api/health || exit 1

# 启动应用
CMD ["./apk-analysis-server"]
