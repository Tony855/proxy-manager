FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder
WORKDIR /app
ARG TARGETARCH
ARG TARGETOS
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN apk add --no-cache git build-base
COPY go.mod ./
COPY go.sum* ./
RUN go mod download
COPY . .
RUN go build -o manager -ldflags="-s -w" main.go

FROM alpine:3.18
ARG TARGETARCH
ENV TARGETARCH=${TARGETARCH:-amd64}

RUN apk update && apk add --no-cache \
    bash curl iptables libc6-compat ca-certificates tzdata wireguard-tools unzip jq \
    && mkdir -p /etc/wireguard

RUN set -eux; \
    case "$TARGETARCH" in \
        "amd64") ARCH_SUFFIX=amd64;; \
        "arm64") ARCH_SUFFIX=arm64;; \
        "arm") ARCH_SUFFIX=armv7;; \
        *) echo "Unsupported arch: $TARGETARCH"; exit 1;; \
    esac; \
    MIHOMO_LATEST=$(curl -sL https://api.github.com/repos/MetaCubeX/mihomo/releases/latest | jq -r '.tag_name'); \
    curl -Lo /tmp/mihomo.gz "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_LATEST}/mihomo-linux-${ARCH_SUFFIX}.gz"; \
    gunzip /tmp/mihomo.gz -c > /usr/local/bin/mihomo; \
    chmod +x /usr/local/bin/mihomo; \
    XRAY_LATEST=$(curl -sL https://api.github.com/repos/XTLS/Xray-core/releases/latest | jq -r '.tag_name'); \
    curl -Lo /tmp/xray.zip "https://github.com/XTLS/Xray-core/releases/download/${XRAY_LATEST}/Xray-linux-${ARCH_SUFFIX}.zip"; \
    unzip /tmp/xray.zip -d /usr/local/bin/; \
    chmod +x /usr/local/bin/xray; \
    SING_BOX_LATEST=$(curl -sL https://api.github.com/repos/SagerNet/sing-box/releases/latest | jq -r '.tag_name'); \
    curl -Lo /tmp/sing-box.tar.gz "https://github.com/SagerNet/sing-box/releases/download/${SING_BOX_LATEST}/sing-box-${SING_BOX_LATEST#v}-linux-${ARCH_SUFFIX}.tar.gz"; \
    tar -xzf /tmp/sing-box.tar.gz -C /usr/local/bin/ --strip-components=1; \
    chmod +x /usr/local/bin/sing-box; \
    rm -rf /tmp/*

WORKDIR /app
COPY --from=builder /app/manager .
COPY entrypoint.sh .
COPY index.html .
RUN chmod +x entrypoint.sh && mkdir -p /app/config /var/log/mihomo /var/run/xray
ENV API_SECRET=your_secret ADMIN_USER=admin ADMIN_PASS=admin123
EXPOSE 8080 9090 53/tcp 53/udp 1080 7890-7892
CMD ["./entrypoint.sh"]
