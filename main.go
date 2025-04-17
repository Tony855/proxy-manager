package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"gopkg.in/yaml.v3"
)

const (
	CoreClash    = "clash"
	CoreXray     = "xray"
	CoreSingBox  = "sing-box"
)

type CoreConfig struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}

type ProxyConfig struct {
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Server   string                 `json:"server"`
	Port     int                    `json:"port"`
	Auth     map[string]interface{} `json:"auth"`
	Network  string                 `json:"network"`
	Security string                 `json:"security"`
	Protocol string                 `json:"protocol"`
	MTU      int                    `json:"mtu"`
}

type DNSConfig struct {
	Nameserver  []string `json:"nameserver"`
	Fallback    []string `json:"fallback"`
}

type IPGroup struct {
	Name   string   `json:"name"`
	IPs    []string `json:"ips"`
	Action string   `json:"action"`
}

type ConfigPayload struct {
	Core     CoreConfig    `json:"core"`
	Proxies  []ProxyConfig `json:"proxies"`
	DNS      DNSConfig     `json:"dns"`
	IPGroups []IPGroup     `json:"ip_groups"`
}

type AuthClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

var jwtSecret = []byte(os.Getenv("API_SECRET"))

func init() {
	debug.SetGCPercent(30)
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "需要认证"})
			return
		}

		token, err := jwt.ParseWithClaims(tokenString, &AuthClaims{}, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效凭证"})
			return
		}

		c.Next()
	}
}

func loginHandler(c *gin.Context) {
	type LoginRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Username != os.Getenv("ADMIN_USER") || req.Password != os.Getenv("ADMIN_PASS") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效凭证"})
		return
	}

	claims := AuthClaims{
		req.Username,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenString})
}

func generateConfig(payload ConfigPayload) ([]byte, error) {
	switch payload.Core.Type {
	case CoreXray:
		return generateXrayConfig(payload)
	case CoreSingBox:
		return generateSingBoxConfig(payload)
	default:
		return generateClashConfig(payload)
	}
}

func generateXrayConfig(payload ConfigPayload) ([]byte, error) {
	xrayConfig := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"inbounds": []map[string]interface{}{
			{
				"port":     1080,
				"protocol": "socks",
				"settings": map[string]interface{}{
					"auth": "noauth",
				},
			},
		},
		"outbounds": []map[string]interface{}{
			{
				"protocol": "freedom",
				"tag":      "direct",
			},
		},
	}

	for _, proxy := range payload.Proxies {
		outbound := make(map[string]interface{})

		switch proxy.Type {
		case "http", "socks5":
			outbound["protocol"] = proxy.Type
			outbound["settings"] = map[string]interface{}{
				"servers": []map[string]interface{}{{
					"address":  proxy.Server,
					"port":     proxy.Port,
					"users": []map[string]interface{}{{
						"user": proxy.Auth["username"],
						"pass": proxy.Auth["password"],
					}},
				}},
			}
		case "hysteria":
			outbound["protocol"] = "hysteria"
			outbound["settings"] = map[string]interface{}{
				"servers": []map[string]interface{}{{
					"host":     proxy.Server,
					"port":     proxy.Port,
					"auth":     proxy.Auth["auth-str"],
					"obfs":     proxy.Auth["obfs-password"],
					"protocol": proxy.Auth["protocol"],
					"up_mbps":  100,
					"down_mbps": 100,
				}},
			}
		case "wireguard":
			outbound["protocol"] = "wireguard"
			outbound["settings"] = map[string]interface{}{
				"secretKey": proxy.Auth["private-key"],
				"peers": []map[string]interface{}{{
					"publicKey":    proxy.Auth["peer-public-key"],
					"preSharedKey": proxy.Auth["preshared-key"],
					"endpoint":    proxy.Auth["peer-address"],
				}},
				"mtu": proxy.MTU,
			}
		case "vmess":
			outbound["protocol"] = "vmess"
			outbound["settings"] = map[string]interface{}{
				"vnext": []map[string]interface{}{{
					"address": proxy.Server,
					"port":    proxy.Port,
					"users": []map[string]interface{}{{
						"id":       proxy.Auth["uuid"],
						"alterId": proxy.Auth["alterId"],
					}},
				}},
			}
		}

		xrayConfig["outbounds"] = append(xrayConfig["outbounds"].([]map[string]interface{}), outbound)
	}

	return json.MarshalIndent(xrayConfig, "", "  ")
}

func generateSingBoxConfig(payload ConfigPayload) ([]byte, error) {
	singBoxConfig := map[string]interface{}{
		"log": map[string]interface{}{
			"level": "info",
		},
		"dns": map[string]interface{}{
			"servers": payload.DNS.Nameserver,
		},
		"inbounds": []map[string]interface{}{
			{
				"type":        "mixed",
				"tag":        "mixed-in",
				"listen":     "::",
				"listen_port": 1080,
			},
		},
		"outbounds": []map[string]interface{}{
			{
				"type": "direct",
				"tag":  "direct",
			},
		},
	}

	for _, proxy := range payload.Proxies {
		outbound := map[string]interface{}{
			"type": proxy.Type,
			"tag":  proxy.Name,
			"server": proxy.Server,
			"server_port": proxy.Port,
		}

		switch proxy.Type {
		case "vmess":
			outbound["uuid"] = proxy.Auth["uuid"]
			outbound["alter_id"] = proxy.Auth["alterId"]
		}

		singBoxConfig["outbounds"] = append(singBoxConfig["outbounds"].([]map[string]interface{}), outbound)
	}

	return json.MarshalIndent(singBoxConfig, "", "  ")
}

func generateClashConfig(payload ConfigPayload) ([]byte, error) {
	tunConfig := map[string]interface{}{
		"enable":                true,
		"stack":                "system",
		"dns-hijack":          []string{"any:53"},
		"auto-route":           true,
		"auto-detect-interface": true,
		"strict-route":         true,
	}

	config := map[string]interface{}{
		"port":              7890,
		"socks-port":        7891,
		"redir-port":        7892,
		"allow-lan":         false,
		"mode":              "Rule",
		"log-level":         "info",
		"external-control": "0.0.0.0:9090",
		"secret":           os.Getenv("API_SECRET"),
		"tun":              tunConfig,
		"dns":              generateDNSConfig(payload.DNS),
		"proxies":          generateProxies(payload.Proxies),
		"proxy-groups":     generateProxyGroups(payload),
		"rules":            generateRules(payload),
	}

	return yaml.Marshal(config)
}

func generateDNSConfig(dns DNSConfig) map[string]interface{} {
	return map[string]interface{}{
		"enable":         true,
		"enhanced-mode": "fake-ip",
		"listen":        "0.0.0.0:53",
		"nameserver":    dns.Nameserver,
		"fallback":      dns.Fallback,
		"fallback-filter": map[string]interface{}{
			"ipcidr": []string{"240.0.0.0/4", "0.0.0.0/32"},
		},
	}
}

func generateProxyGroups(payload ConfigPayload) []map[string]interface{} {
	groups := []map[string]interface{}{
		{
			"name":     "auto-route",
			"type":     "url-test",
			"url":      "http://www.gstatic.com/generate_204",
			"interval": 300,
			"proxies":  getProxyNames(payload.Proxies),
		},
	}

	for _, group := range payload.IPGroups {
		groupConfig := map[string]interface{}{
			"name":    group.Name,
			"type":    "select",
			"proxies": []string{"auto-route", "DIRECT"},
		}

		switch group.Action {
		case "direct":
			groupConfig["proxies"] = []string{"DIRECT"}
		case "block":
			groupConfig["proxies"] = []string{"REJECT"}
		}

		groups = append(groups, groupConfig)
	}

	return groups
}

func generateRules(payload ConfigPayload) []string {
	var rules []string

	for _, group := range payload.IPGroups {
		for _, ip := range group.IPs {
			if _, _, err := net.ParseCIDR(ip); err != nil {
				continue
			}

			rule := fmt.Sprintf("IP-CIDR,%s,%s", ip, group.Name)
			rules = append(rules, rule)
		}
	}

	rules = append(rules, "MATCH,auto-route")
	return rules
}

func getProxyNames(proxies []ProxyConfig) []string {
	names := make([]string, 0, len(proxies))
	for _, p := range proxies {
		names = append(names, p.Name)
	}
	return names
}

func generateProxies(proxyCfgs []ProxyConfig) []map[string]interface{} {
	proxies := make([]map[string]interface{}, 0, len(proxyCfgs))
	for _, p := range proxyCfgs {
		proxy := map[string]interface{}{
			"name":     p.Name,
			"type":     p.Type,
			"server":   p.Server,
			"port":     p.Port,
			"udp":      true,
			"fast-open": true,
		}

		switch p.Type {
		case "ss":
			proxy["cipher"] = p.Auth["cipher"]
			proxy["password"] = p.Auth["password"]
		case "vmess":
			proxy["uuid"] = p.Auth["uuid"]
			proxy["alterId"] = p.Auth["alterId"]
			proxy["network"] = p.Network
			proxy["tls"] = p.Security == "tls"
		case "vless":
			proxy["uuid"] = p.Auth["uuid"]
			proxy["flow"] = p.Auth["flow"]
			proxy["tls"] = p.Security == "tls"
		case "trojan":
			proxy["password"] = p.Auth["password"]
			proxy["sni"] = p.Auth["sni"]
		case "http", "socks5":
			proxy["username"] = p.Auth["username"]
			proxy["password"] = p.Auth["password"]
		case "hysteria":
			proxy["protocol"] = p.Auth["protocol"]
			proxy["auth_str"] = p.Auth["auth-str"]
			proxy["obfs"] = p.Auth["obfs-password"]
		case "hysteria2":
			proxy["password"] = p.Auth["password"]
			proxy["obfs"] = map[string]interface{}{
				"type":     p.Auth["obfs"],
				"password": p.Auth["obfs-password"],
			}
		case "tuic":
			proxy["uuid"] = p.Auth["uuid"]
			proxy["password"] = p.Auth["password"]
			proxy["congestion_control"] = p.Auth["congestion-control"]
			proxy["udp_relay_mode"] = "native"
		case "wireguard":
			proxy["private-key"] = p.Auth["private-key"]
			proxy["peers"] = []map[string]interface{}{{
				"public-key":    p.Auth["peer-public-key"],
				"pre-shared-key": p.Auth["preshared-key"],
				"endpoint":      p.Auth["peer-address"],
			}}
			proxy["mtu"] = p.MTU
		}

		proxies = append(proxies, proxy)
	}
	return proxies
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("X-Proxy-Manager-Version", "2.2")
		c.Next()
	})

	r.POST("/api/login", loginHandler)

	api := r.Group("/api")
	api.Use(authMiddleware())
	{
		api.POST("/config", func(c *gin.Context) {
			var payload ConfigPayload
			if err := c.ShouldBindJSON(&payload); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			for _, group := range payload.IPGroups {
				for _, ip := range group.IPs {
					if _, _, err := net.ParseCIDR(ip); err != nil {
						c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的IP格式: %s", ip)})
						return
					}
				}
			}

			configData, err := generateConfig(payload)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			if err := os.MkdirAll("/app/config", 0755); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			configPath := filepath.Join("/app/config", "config.yaml")
			if err := ioutil.WriteFile(configPath, configData, 0644); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			cmd := exec.Command("curl", "-XPUT",
				"-H", fmt.Sprintf("Authorization: Bearer %s", os.Getenv("API_SECRET")),
				"http://localhost:9090/configs",
				"-d", fmt.Sprintf(`{"path": "%s", "force": true}`, configPath))

			if output, err := cmd.CombinedOutput(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":  "配置重载失败",
					"detail": string(output),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

		api.GET("/verify", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
	}

	if err := r.Run(":8080"); err != nil {
		panic(fmt.Sprintf("服务器启动失败: %v", err))
	}
}