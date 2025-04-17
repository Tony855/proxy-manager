# 构建阶段
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder
WORKDIR /app

# 自动识别目标架构
ARG TARGETARCH
ARG TARGETOS

# 配置Go环境
ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH

# 安装编译依赖
RUN apk add --no-cache git build-base

# 复制模块文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建可执行文件
RUN go build -o manager -ldflags="-s -w" main.go

# 运行时阶段
FROM alpine:3.18

# 自动识别架构
ARG TARGETARCH
ENV ARCH=${TARGETARCH:-amd64}

# 安装运行时依赖
RUN apk update && apk add --no-cache \
    bash \
    curl \
    iptables \
    libc6-compat \
    ca-certificates \
    tzdata \
    wireguard-tools \
    unzip \
    jq \
    && mkdir -p /etc/wireguard

# 架构映射表
RUN case "$ARCH" in \
    "amd64") ARCH_SUFFIX=amd64;; \
    "arm64") ARCH_SUFFIX=arm64;; \
    "arm/v7") ARCH_SUFFIX=armv7;; \
    *) echo "Unsupported architecture: $ARCH"; exit 1;; \
    esac

# 安装核心组件
RUN set -eux; \
    # 安装Mihomo
    MIHOMO_LATEST=$(curl -sL https://api.github.com/repos/MetaCubeX/mihomo/releases/latest | jq -r '.tag_name'); \
    curl -Lo /tmp/mihomo.gz "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_LATEST}/mihomo-${ARCH_SUFFIX}.gz"; \
    gunzip /tmp/mihomo.gz -c > /usr/local/bin/mihomo; \
    chmod +x /usr/local/bin/mihomo; \
    \
    # 安装Xray-core
    XRAY_LATEST=$(curl -sL https://api.github.com/repos/XTLS/Xray-core/releases/latest | jq -r '.tag_name'); \
    curl -Lo /tmp/xray.zip "https://github.com/XTLS/Xray-core/releases/download/${XRAY_LATEST}/Xray-linux-${ARCH_SUFFIX}.zip"; \
    unzip /tmp/xray.zip -d /usr/local/bin/; \
    chmod +x /usr/local/bin/xray; \
    \
    # 安装sing-box
    SING_BOX_LATEST=$(curl -sL https://api.github.com/repos/SagerNet/sing-box/releases/latest | jq -r '.tag_name'); \
    curl -Lo /tmp/sing-box.tar.gz "https://github.com/SagerNet/sing-box/releases/download/${SING_BOX_LATEST}/sing-box-${SING_BOX_LATEST#v}-linux-${ARCH_SUFFIX}.tar.gz"; \
    tar -xzf /tmp/sing-box.tar.gz -C /usr/local/bin/ --strip-components=1; \
    chmod +x /usr/local/bin/sing-box; \
    \
    # 清理缓存
    rm -rf /tmp/*

# 设置工作目录
WORKDIR /app

# 复制文件
COPY --from=builder /app/manager .
COPY entrypoint.sh .
COPY index.html .

# 设置权限
RUN chmod +x entrypoint.sh && \
    mkdir -p /app/config && \
    mkdir -p /var/log/mihomo && \
    mkdir -p /var/run/xray

# 环境变量
ENV API_SECRET=v2netcc \
    ADMIN_USER=admin \
    ADMIN_PASS=admin123

# 暴露端口
EXPOSE 8080 9090 53/tcp 53/udp 1080 7890-7892

# 启动脚本
CMD ["./entrypoint.sh"]
