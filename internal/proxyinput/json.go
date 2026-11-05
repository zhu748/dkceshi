package proxyinput

import (
	"ds2api/internal/config"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Reject duplicate keys instead of silently replacing credentials or TLS options.
func DecodeJSON(text string) (any, error) {
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 24 {
			return nil, errors.New("JSON 嵌套过深")
		}
		t, e := d.Token()
		if e != nil {
			return nil, errors.New("JSON 格式无效")
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				m := map[string]any{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return nil, errors.New("JSON 格式无效")
					}
					key, ok := k.(string)
					if !ok {
						return nil, errors.New("JSON 字段无效")
					}
					if _, exists := m[key]; exists {
						return nil, errors.New("JSON 含重复字段")
					}
					v, e := read(depth + 1)
					if e != nil {
						return nil, e
					}
					m[key] = v
				}
				_, e = d.Token()
				return m, e
			case '[':
				var a []any
				for d.More() {
					v, e := read(depth + 1)
					if e != nil {
						return nil, e
					}
					a = append(a, v)
				}
				_, e = d.Token()
				return a, e
			}
			return nil, errors.New("JSON 格式无效")
		}
		return t, nil
	}
	v, e := read(0)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errors.New("JSON 存在多余内容")
	}
	return v, nil
}

func objectProxy(m map[string]any, selected string) (config.Proxy, error) {
	var p config.Proxy
	allowed := map[string]bool{"type": true, "scheme": true, "host": true, "server": true, "port": true, "server_port": true, "username": true, "password": true, "name": true, "options": true, "tls_server_name": true, "ca_file": true, "insecure": true, "realm": true, "mtu": true}
	for k := range m {
		if !allowed[k] {
			return p, errors.New("节点 JSON 含未知字段")
		}
	}
	for _, pair := range [][2]string{{"type", "scheme"}, {"host", "server"}, {"port", "server_port"}} {
		if _, ok := m[pair[0]]; ok {
			if _, exists := m[pair[1]]; exists {
				return p, errors.New("节点 JSON 含重复别名")
			}
		}
	}
	raw := map[string]any{}
	for k, v := range m {
		raw[k] = v
	}
	for _, pair := range [][2]string{{"type", "scheme"}, {"host", "server"}, {"port", "server_port"}} {
		if v, ok := m[pair[1]]; ok {
			raw[pair[0]] = v
		}
	}
	p.Type = selected
	for k, dest := range map[string]*string{"type": &p.Type, "host": &p.Host, "username": &p.Username, "password": &p.Password, "name": &p.Name} {
		if v, ok := raw[k]; ok {
			s, ok := v.(string)
			if !ok {
				return p, errors.New("节点文本字段必须为字符串")
			}
			*dest = s
		}
	}
	p.Type = config.NormalizeProxyType(p.Type)
	if p.Type == "" {
		p.Type = "socks5h"
	}
	p.Port = config.ProxyDefaultPort(p.Type)
	if v, ok := raw["port"]; ok {
		n, ok := v.(json.Number)
		if !ok {
			return p, errors.New("节点端口必须为整数")
		}
		i, e := n.Int64()
		if e != nil || i < 1 || i > 65535 {
			return p, errors.New("节点端口无效")
		}
		p.Port = int(i)
	}
	opts, err := objectOptions(m)
	if err != nil {
		return p, err
	}
	q := makeQuery(opts)
	if err := applyQuery(&p, q); err != nil {
		return p, err
	}
	return finish(p)
}
