FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
# 安装编译依赖（修正build-base拼写）
RUN apk add --no-cache git build-base
RUN go build -o manager -ldflags="-s -w" main.go

FROM alpine:3.18
# 分步骤安装依赖（修正包名并添加更新步骤）
RUN apk update && apk add --no-cache \
    bash \
    curl \
    iptables \
    libc6-compat \
    ca-certificates \  # 修正拼写
    tzdata \
    wireguard-tools \
    unzip \            # 添加解压工具
    && mkdir -p /etc/wireguard

# 安装核心组件（修正下载命令）
RUN curl -Lo /tmp/mihomo.gz https://github.com/MetaCubeX/mihomo/releases/download/v1.18.0/mihomo-linux-amd64-v1.18.0.gz \
    && gunzip /tmp/mihomo.gz -c > /usr/local/bin/mihomo \
    && chmod +x /usr/local/bin/mihomo

RUN curl -Lo /tmp/xray.zip https://github.com/XTLS/Xray-core/releases/download/v1.8.6/Xray-linux-64.zip \
    && unzip /tmp/xray.zip -d /usr/local/bin/ \
    && chmod +x /usr/local/bin/xray

RUN curl -Lo /tmp/sing-box.tar.gz https://github.com/SagerNet/sing-box/releases/download/v1.8.0/sing-box-1.8.0-linux-amd64.tar.gz \
    && tar -xzf /tmp/sing-box.tar.gz -C /usr/local/bin/ --strip-components=1 \
    && chmod +x /usr/local/bin/sing-box

WORKDIR /app
COPY --from=builder /app/manager .
COPY entrypoint.sh .
COPY index.html .

RUN chmod +x entrypoint.sh \
    && mkdir -p /app/config \
    && mkdir -p /var/log/mihomo \
    && mkdir -p /var/run/xray

ENV API_SECRET=your_api_secret_here
ENV ADMIN_USER=admin
ENV ADMIN_PASS=admin123

EXPOSE 8080 9090 53/tcp 53/udp 1080 7890-7892
CMD ["./entrypoint.sh"]
