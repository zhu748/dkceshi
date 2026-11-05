package proxyinput

import (
	"ds2api/internal/config"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

func applyQuery(p *config.Proxy, raw string) error {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return errors.New("代理参数编码无效")
	}
	seen := map[string]bool{}
	for key, values := range q {
		if len(values) != 1 {
			return errors.New("代理参数重复")
		}
		v := values[0]
		alias := key
		switch key {
		case "user":
			alias = "username"
		case "pass":
			alias = "password"
		case "tls_server_name":
			alias = "sni"
		case "ca_file":
			alias = "ca"
		}
		if seen[alias] {
			return errors.New("代理参数重复")
		}
		seen[alias] = true
		if (alias == "sni" || alias == "ca" || alias == "insecure") && p.Type != "https" && p.Type != "turns" && p.Type != "sstp" {
			return errors.New("TLS 参数仅适用于 HTTPS/TURNS/SSTP")
		}
		if alias == "realm" && p.Type != "turn" && p.Type != "turns" {
			return errors.New("realm 仅适用于 TURN/TURNS")
		}
		switch alias {
		case "username":
			if p.Username != "" {
				return errors.New("代理用户名重复")
			}
			p.Username = v
		case "password":
			if p.Password != "" {
				return errors.New("代理密码重复")
			}
			p.Password = v
		case "transport":
			if (p.Type != "turn" && p.Type != "turns") || strings.ToLower(v) != "tcp" {
				return errors.New("TURN 仅支持 RFC 6062 TCP 中继，transport 必须为 tcp")
			}
		case "sni":
			p.ServerName = v
		case "ca":
			p.CAFile = v
		case "insecure":
			if v != "true" && v != "false" && v != "1" && v != "0" {
				return errors.New("insecure 必须为 true/false 或 1/0")
			}
			p.Insecure = v == "true" || v == "1"
		case "realm":
			p.Realm = v
		case "mtu":
			p.MTU, err = strconv.Atoi(v)
			if err != nil || p.MTU == 0 {
				return errors.New("SSTP MTU 无效")
			}
		default:
			return errors.New("代理包含不支持的参数")
		}
	}
	return nil
}

func makeQuery(opts map[string]string) string {
	q := url.Values{}
	for k, v := range opts {
		q.Set(k, v)
	}
	return q.Encode()
}
