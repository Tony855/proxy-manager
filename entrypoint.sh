#!/bin/bash

# 优化内核参数
sysctl -w net.core.rmem_max=2500000
sysctl -w net.core.wmem_max=2500000
sysctl -w net.ipv4.tcp_rmem="4096 87380 2500000"
sysctl -w net.ipv4.tcp_wmem="4096 65536 2500000"
sysctl -w net.ipv4.tcp_mtu_probing=1
sysctl -w net.ipv4.tcp_fastopen=3
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535

# 创建虚拟网络设备
mkdir -p /dev/net
mknod /dev/net/tun c 10 200
chmod 0666 /dev/net/tun

# 启动服务
/app/manager &

# 根据配置选择核心
if [ -f "/app/config/config.yaml" ]; then
    case $(grep '^core:' /app/config/config.yaml | awk '{print $2}') in
        "clash")
            /usr/local/bin/mihomo -d /app/config
            ;;
        "xray")
            /usr/local/bin/xray run -config /app/config/config.json
            ;;
        "sing-box")
            /usr/local/bin/sing-box run -c /app/config/config.json
            ;;
        *)
            echo "Using default core: Clash Meta"
            /usr/local/bin/mihomo -d /app/config
            ;;
    esac
fi

# 保持容器运行
tail -f /dev/null