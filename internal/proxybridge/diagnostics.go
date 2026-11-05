package proxybridge

import (
	"strings"
	"sync"
	"time"
)

var helperMessages = map[string]string{
	"server_connect_failed":                                    "无法连接代理服务器",
	"server_tls_failed_check_ca_sni":                           "代理 TLS 握手失败，请检查证书、CA 和 SNI",
	"ca_read_failed":                                           "无法读取代理 CA 文件",
	"ca_invalid_pem":                                           "代理 CA 文件格式无效",
	"sstp_auth_failed_requires_mschapv2":                       "SSTP 认证失败，请检查用户名、密码及 MS-CHAPv2 支持",
	"sstp_handshake_timeout":                                   "SSTP 握手超时",
	"sstp_handshake_failed_check_tls_credentials_and_mschapv2": "SSTP 握手失败，请检查 TLS 和 MS-CHAPv2 支持",
	"target_dns_failed":                                        "代理转换器无法解析目标域名",
	"turn_allocate_failed_check_credentials_and_rfc6062":       "TURN 分配失败，请检查凭据及 RFC 6062 TCP 中继支持",
	"turn_connect_failed_check_peer_policy":                    "TURN 目标连接失败，请检查出口策略",
	"turn_target_connect_failed":                               "TURN 分配或目标连接失败，请检查 TCP 中继支持和出口策略",
	"sstp_target_connect_failed":                               "SSTP 目标连接失败，请检查 VPN 出口策略",
}

type helperDiagnostics struct {
	mu      sync.Mutex
	pending string
	message string
	updated time.Time
}

func (d *helperDiagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, b := range p {
		if b == '\n' {
			if code, ok := strings.CutPrefix(d.pending, "DS2API_PROXY_ERROR "); ok {
				if message := helperMessages[code]; message != "" {
					d.message = message
					d.updated = time.Now()
				}
			}
			d.pending = ""
		} else if len(d.pending) < 512 {
			d.pending += string(b)
		}
	}
	return len(p), nil
}

func (d *helperDiagnostics) recentFailure(since time.Time, fallback string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.updated.Before(since) && d.message != "" {
		return fallback + "；转换器最近诊断：" + d.message
	}
	return fallback
}

func (d *helperDiagnostics) failure(fallback string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.message != "" {
		return d.message
	}
	return fallback
}
