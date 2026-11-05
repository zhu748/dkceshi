package config

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ProxyTypes = []string{"http", "https", "socks4", "socks4a", "socks5", "socks5h", "turn", "turns", "sstp"}

func ProxyDefaultPort(kind string) int {
	switch NormalizeProxyType(kind) {
	case "http":
		return 80
	case "https", "sstp":
		return 443
	case "turn":
		return 3478
	case "turns":
		return 5349
	default:
		return 1080
	}
}

// ProxyIdentity includes connection settings and credentials, but no display name or ID.
func ProxyIdentity(p Proxy) string {
	p = NormalizeProxy(p)
	p.ID, p.Name = "", ""
	p.Host = strings.ToLower(p.Host)
	if ip := net.ParseIP(p.Host); ip != nil {
		p.Host = ip.String()
	}
	p.ServerName = strings.ToLower(p.ServerName)
	if p.ServerName == p.Host {
		p.ServerName = ""
	}
	if p.Type == "sstp" && p.MTU >= 1400 && p.MTU <= 9000 {
		p.MTU = 0
	}
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ValidateProxyConfig(proxies []Proxy) error {
	seen := make(map[string]bool, len(proxies))
	for _, p := range proxies {
		p = NormalizeProxy(p)
		for _, v := range []string{p.ID, p.Host, p.Username, p.Password, p.Name, p.Realm, p.ServerName, p.CAFile} {
			if !utf8.ValidString(v) {
				return fmt.Errorf("代理字段必须为有效 UTF-8 文本")
			}
		}
		if err := ValidateTrimmedString("proxies.id", p.ID, true); err != nil {
			return err
		}
		supported := false
		for _, kind := range ProxyTypes {
			supported = supported || p.Type == kind
		}
		if !supported {
			return fmt.Errorf("不支持的代理协议，支持 HTTP/HTTPS/SOCKS4/SOCKS4A/SOCKS5/SOCKS5H/TURN/TURNS/SSTP")
		}
		if p.Host == "" || strings.ContainsAny(p.Host, "/@?#\\%[]") || strings.ContainsFunc(p.Host, unicode.IsSpace) || strings.ContainsFunc(p.Host, unicode.IsControl) || (strings.Contains(p.Host, ":") && net.ParseIP(p.Host) == nil) {
			return fmt.Errorf("代理主机无效")
		}
		if err := ValidateIntRange("proxies.port", p.Port, 1, 65535, true); err != nil {
			return err
		}
		if strings.ContainsFunc(p.Username+p.Password+p.Name+p.Realm+p.ServerName+p.CAFile, unicode.IsControl) {
			return fmt.Errorf("代理字段不能包含控制字符")
		}
		if p.Type == "sstp" && (p.Username == "" || p.Password == "") {
			return fmt.Errorf("SSTP 需要用户名和密码（MS-CHAPv2）")
		}
		if (p.Type == "socks4" || p.Type == "socks4a") && p.Password != "" {
			return fmt.Errorf("SOCKS4/4A 只支持用户 ID，不支持密码认证，请清空密码或选择 SOCKS5")
		}
		if p.Password != "" && p.Username == "" {
			return fmt.Errorf("填写代理密码时也需要填写用户名")
		}
		if (p.Type == "socks5" || p.Type == "socks5h") && (len(p.Username) > 255 || len(p.Password) > 255) {
			return fmt.Errorf("SOCKS5 凭据不能超过 255 字节")
		}
		tls := p.Type == "https" || p.Type == "turns" || p.Type == "sstp"
		if !tls && (p.ServerName != "" || p.CAFile != "" || p.Insecure) {
			return fmt.Errorf("TLS 设置仅适用于 HTTPS/TURNS/SSTP")
		}
		if p.ServerName != "" && (strings.ContainsAny(p.ServerName, "/@?#\\%[]") || strings.ContainsFunc(p.ServerName, unicode.IsSpace)) {
			return fmt.Errorf("SNI 无效")
		}
		if p.Realm != "" && p.Type != "turn" && p.Type != "turns" {
			return fmt.Errorf("realm 仅适用于 TURN/TURNS")
		}
		if p.MTU != 0 && (p.Type != "sstp" || p.MTU < 576 || p.MTU > 9000) {
			return fmt.Errorf("SSTP MTU 必须为 576–9000")
		}
		if seen[p.ID] {
			return fmt.Errorf("duplicate proxy id: %s", p.ID)
		}
		seen[p.ID] = true
	}
	return nil
}

// PublicProxy never includes the saved password.
func PublicProxy(p Proxy) map[string]any {
	p = NormalizeProxy(p)
	return map[string]any{"id": p.ID, "name": p.Name, "type": p.Type, "host": p.Host, "port": p.Port, "username": p.Username, "has_password": p.Password != "", "tls_server_name": p.ServerName, "ca_file": p.CAFile, "insecure": p.Insecure, "realm": p.Realm, "mtu": p.MTU}
}

type Proxy struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	ServerName string `json:"tls_server_name,omitempty"`
	CAFile     string `json:"ca_file,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"`
	Realm      string `json:"realm,omitempty"`
	MTU        int    `json:"mtu,omitempty"`
}

func NormalizeProxy(p Proxy) Proxy {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Type = NormalizeProxyType(p.Type)
	p.Host = strings.TrimSpace(p.Host)
	if strings.HasPrefix(p.Host, "[") && strings.HasSuffix(p.Host, "]") {
		inside := p.Host[1 : len(p.Host)-1]
		if strings.Contains(inside, ":") && net.ParseIP(inside) != nil {
			p.Host = inside
		}
	}
	p.ServerName = strings.TrimSpace(p.ServerName)
	p.CAFile = strings.TrimSpace(p.CAFile)
	if p.ID == "" {
		p.ID = StableProxyID(p)
	}
	if p.Name == "" && p.Host != "" && p.Port > 0 {
		p.Name = net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	}
	return p
}

// Keep legacy generated IDs stable for configurations that omit IDs.
func StableProxyID(p Proxy) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(p.Type)) + "|" + strings.ToLower(strings.TrimSpace(p.Host)) + "|" + fmt.Sprintf("%d", p.Port) + "|" + strings.TrimSpace(p.Username)))
	return "proxy_" + hex.EncodeToString(sum[:6])
}

func NormalizeProxyType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "socks":
		return "socks5"
	case "socks4-hostname":
		return "socks4a"
	case "socks5-hostname":
		return "socks5h"
	}
	return value
}
