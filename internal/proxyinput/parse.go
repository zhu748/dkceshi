// Package proxyinput parses proxy descriptions without making network requests.
package proxyinput

import (
	"ds2api/internal/config"
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var ErrCredentialsRequired = errors.New("SSTP 需要补齐用户名和密码（MS-CHAPv2）")

var explicitScheme = regexp.MustCompile(`(?i)^([a-z][a-z0-9+.-]*)://`)
var supplierRecord = regexp.MustCompile(`^(\[[^\]]+\]|[^:@/?#\s]+):([0-9]+):([^:]+):(.+)$`)

func Parse(value, selected string) (config.Proxy, error) {
	value = strings.TrimSpace(value)
	p := config.Proxy{Type: config.NormalizeProxyType(selected)}
	if strings.ContainsFunc(value, unicode.IsControl) || value == "" {
		return p, errors.New("代理地址为空或含控制字符")
	}
	lower := strings.ToLower(value)
	for _, s := range []string{"turn:", "turns:"} {
		if strings.HasPrefix(lower, s) && !strings.HasPrefix(lower, s+"//") {
			value = value[:len(s)] + "//" + value[len(s):]
			break
		}
	}
	if m := explicitScheme.FindStringSubmatch(value); m != nil {
		p.Type = strings.ToLower(m[1])
		value = value[len(m[0]):]
	}
	if p.Type == "" {
		p.Type = "socks5h"
	}
	p.Type = config.NormalizeProxyType(p.Type)
	// Literal four-field formats are recognized before URL punctuation in credentials.
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == '|' || unicode.IsSpace(r) })
	if len(fields) == 4 && (strings.Contains(value, "|") || strings.ContainsAny(value, " \t")) {
		p.Host, p.Username, p.Password = fields[0], fields[2], fields[3]
		port, err := strconv.Atoi(fields[1])
		if err != nil {
			return p, errors.New("代理端口无效")
		}
		p.Port = port
		return finish(p)
	}
	// Supplier host:port:user:password permits reserved characters in the password.
	if m := supplierRecord.FindStringSubmatch(value); m != nil && (!strings.Contains(value, "@") || !validEndpoint(value[strings.LastIndex(value, "@")+1:])) && (looksHost(m[1]) || !validEndpoint(m[3]+":"+m[4])) {
		p.Host = m[1]
		p.Port, _ = strconv.Atoi(m[2])
		var err error
		p.Username, err = url.PathUnescape(m[3])
		if err != nil {
			return p, errors.New("凭据 URL 编码无效")
		}
		password, query, hasQuery := strings.Cut(m[4], "?")
		p.Password, err = url.PathUnescape(password)
		if err != nil {
			return p, errors.New("凭据 URL 编码无效")
		}
		if hasQuery {
			u, e := url.Parse("?" + query)
			if e != nil {
				return p, errors.New("代理参数无效")
			}
			if e = applyQuery(&p, u.RawQuery); e != nil {
				return p, e
			}
			p.Name = u.Fragment
		}

		return finish(p)
	}
	endpoint, suffix := value, ""
	if i := strings.IndexAny(endpoint, "?#"); i >= 0 {
		endpoint, suffix = endpoint[:i], endpoint[i:]
	}
	if i := strings.LastIndex(endpoint, "@"); i >= 0 {
		left, right := endpoint[:i], endpoint[i+1:]
		auth := left
		endpoint = right
		if validEndpoint(left) && (!validEndpoint(right) || (looksHost(endpointHost(left)) && !looksHost(endpointHost(right)))) {
			endpoint, auth = left, right
		}
		if !strings.Contains(auth, ":") {
			for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
				if b, e := enc.DecodeString(auth); e == nil && strings.Contains(string(b), ":") {
					auth = url.PathEscape(string(b))
					auth = strings.Replace(auth, "%3A", ":", 1)
					break
				}
			}
		}
		user, pass, _ := strings.Cut(auth, ":")
		var err error
		p.Username, err = url.PathUnescape(user)
		if err != nil {
			return p, errors.New("凭据 URL 编码无效")
		}
		p.Password, err = url.PathUnescape(pass)
		if err != nil {
			return p, errors.New("凭据 URL 编码无效")
		}
	} else {
		parts := splitColonFields(endpoint)
		if len(parts) == 4 {
			if validPort(parts[1]) && (looksHost(parts[0]) || !validPort(parts[3]) || !looksHost(parts[2])) {
				endpoint = parts[0] + ":" + parts[1]
				p.Username, p.Password = parts[2], parts[3]
			} else {
				endpoint = parts[2] + ":" + parts[3]
				p.Username, p.Password = parts[0], parts[1]
			}
			var err error
			p.Username, err = url.PathUnescape(p.Username)
			if err != nil {
				return p, errors.New("凭据 URL 编码无效")
			}
			p.Password, err = url.PathUnescape(p.Password)
			if err != nil {
				return p, errors.New("凭据 URL 编码无效")
			}
		}
	}
	u, err := url.Parse(p.Type + "://" + endpoint + suffix)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") {
		return p, errors.New("无法识别代理地址，请检查主机、端口和凭据格式")
	}
	p.Host = u.Hostname()
	p.Port = config.ProxyDefaultPort(p.Type)
	if strings.HasSuffix(u.Host, ":") {
		return p, errors.New("代理端口无效")
	}
	if u.Port() != "" {
		p.Port, err = strconv.Atoi(u.Port())
		if err != nil {
			return p, errors.New("代理端口无效")
		}
	}
	p.Name, err = url.PathUnescape(u.EscapedFragment())
	if err != nil {
		return p, errors.New("备注编码无效")
	}
	if err = applyQuery(&p, u.RawQuery); err != nil {
		return p, err
	}
	return finish(p)
}
func finish(p config.Proxy) (config.Proxy, error) {
	p = config.NormalizeProxy(p)
	p.ID = "proxy_" + config.ProxyIdentity(p)[:24]
	if p.Type == "sstp" && (p.Username == "" || p.Password == "") {
		complete := p
		if complete.Username == "" {
			complete.Username = "pending"
		}
		if complete.Password == "" {
			complete.Password = "pending"
		}
		if err := config.ValidateProxyConfig([]config.Proxy{complete}); err != nil {
			return p, err
		}
		return p, ErrCredentialsRequired
	}
	return p, config.ValidateProxyConfig([]config.Proxy{p})
}
func validPort(v string) bool      { n, e := strconv.Atoi(v); return e == nil && n > 0 && n < 65536 }
func endpointHost(v string) string { h, _, _ := net.SplitHostPort(v); return h }
func validEndpoint(v string) bool {
	h, p, e := net.SplitHostPort(v)
	return e == nil && h != "" && validPort(p)
}
func looksHost(v string) bool {
	return net.ParseIP(strings.Trim(v, "[]")) != nil || strings.Contains(v, ".") || v == "localhost"
}
func splitColonFields(v string) []string {
	var fields []string
	start, depth := 0, 0
	for i, r := range v {
		if r == '[' {
			depth++
		}
		if r == ']' {
			depth--
		}
		if r == ':' && depth == 0 {
			fields = append(fields, v[start:i])
			start = i + 1
		}
	}
	return append(fields, v[start:])
}
