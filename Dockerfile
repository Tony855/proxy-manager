FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN apk add --no-cache git build-base
RUN go build -o manager -ldflags="-s -w" main.go

FROM alpine:3.18
RUN apk add --no-cache \
    bash \
    curl \
    iptables \
    libc6-compat \
    ca-certificates \
    tzdata \
    wireguard-tools \
    && mkdir -p /etc/wireguard \
    # 安装核心组件
    && curl -Lo /usr/local/bin/mihomo https://github.com/MetaCubeX/mihomo/releases/download/v1.18.0/mihomo-linux-amd64-v1.18.0.gz \
    && gunzip /usr/local/bin/mihomo \
    && chmod +x /usr/local/bin/mihomo \
    && curl -Lo /usr/local/bin/xray https://github.com/XTLS/Xray-core/releases/download/v1.8.6/Xray-linux-64.zip \
    && unzip /usr/local/bin/xray -d /usr/local/bin/ \
    && chmod +x /usr/local/bin/xray \
    && curl -Lo /usr/local/bin/sing-box https://github.com/SagerNet/sing-box/releases/download/v1.8.0/sing-box-1.8.0-linux-amd64.tar.gz \
    && tar -xzf /usr/local/bin/sing-box -C /usr/local/bin/ \
    && chmod +x /usr/local/bin/sing-box

WORKDIR /app
COPY --from=builder /app/manager .
COPY entrypoint.sh .
COPY index.html .

RUN chmod +x entrypoint.sh \
    && mkdir -p /app/config \
    # 创建必要的运行时目录
    && mkdir -p /var/log/mihomo \
    && mkdir -p /var/run/xray

ENV API_SECRET=your_api_secret_here
ENV ADMIN_USER=admin
ENV ADMIN_PASS=admin123

EXPOSE 8080 9090 53/tcp 53/udp 1080 7890-7892
CMD ["./entrypoint.sh"]