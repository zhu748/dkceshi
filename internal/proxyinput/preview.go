package proxyinput

import (
	"ds2api/internal/config"
	"errors"
	"strings"
)

const MaxItems = 200
const MaxBytes = 256 * 1024

type Row struct {
	Source  string        `json:"source"`
	Status  string        `json:"status"`
	Message string        `json:"message,omitempty"`
	Proxy   *config.Proxy `json:"proxy,omitempty"`
}
type candidate struct {
	source string
	value  any
}

func Preview(text, selected string, existing []config.Proxy) ([]Row, error) {
	if len(text) > MaxBytes {
		return nil, errors.New("导入内容不能超过 256 KiB")
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("请粘贴代理节点")
	}
	candidates, err := collectCandidates(text)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, p := range existing {
		seen[config.ProxyIdentity(p)] = true
	}
	rows := make([]Row, 0, len(candidates))
	for _, c := range candidates {
		row := Row{Source: c.source, Status: "invalid"}
		value := c.value
		var p config.Proxy
		var err error
		switch v := value.(type) {
		case string:
			if looksJSON(v) {
				_, err = DecodeJSON(v)
				if err == nil {
					err = errors.New("JSON 节点格式无效")
				}
				break
			}
			lower := strings.ToLower(v)
			if strings.HasPrefix(lower, "stun:") || strings.HasPrefix(lower, "stuns:") {
				row.Status = "skipped"
				row.Message = "STUN 无法作为 TCP 代理"
				rows = append(rows, row)
				continue
			}
			p, err = Parse(v, selected)
		case map[string]any:
			if _, ok := v["urls"]; ok {
				p, err = parseICE(v)
			} else if _, ok := v["url"]; ok {
				p, err = parseICE(v)
			} else {
				p, err = objectProxy(v, selected)
			}
		default:
			err = errors.New("节点必须是地址字符串或节点对象")
		}
		if err != nil {
			row.Message = err.Error()
			if errors.Is(err, errSTUN) {
				row.Status = "skipped"
			}
			if errors.Is(err, ErrCredentialsRequired) {
				row.Status = "incomplete"
				row.Proxy = &p
			}
		} else {
			row.Proxy = &p
			identity := config.ProxyIdentity(p)
			if seen[identity] {
				row.Status = "duplicate"
				row.Message = "节点已存在，已跳过"
			} else {
				seen[identity] = true
				row.Status = "ready"
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

var errSTUN = errors.New("STUN 无法作为 TCP 代理")

func parseICE(m map[string]any) (config.Proxy, error) {
	var p config.Proxy
	for k := range m {
		if k != "urls" && k != "url" && k != "username" && k != "credential" && k != "credentialType" {
			return p, errors.New("ICE 配置含未知字段")
		}
	}
	if _, ok := m["urls"]; ok {
		if _, exists := m["url"]; exists {
			return p, errors.New("ICE 地址字段重复")
		}
	}
	value, ok := m["urls"]
	if !ok {
		value = m["url"]
	}
	uri, ok := value.(string)
	if !ok {
		return p, errors.New("ICE urls 必须为地址字符串")
	}
	if t, ok := m["credentialType"]; ok && t != "password" {
		return p, errors.New("ICE 只支持 password 凭据")
	}
	lower := strings.ToLower(uri)
	if strings.HasPrefix(lower, "stun:") || strings.HasPrefix(lower, "stuns:") {
		return p, errSTUN
	}
	if !strings.HasPrefix(lower, "turn:") && !strings.HasPrefix(lower, "turns:") {
		return p, errors.New("ICE 只支持 TURN/TURNS")
	}
	p, err := Parse(uri, "turn")
	if err != nil {
		return p, err
	}
	for k, dest := range map[string]*string{"username": &p.Username, "credential": &p.Password} {
		if v, exists := m[k]; exists {
			s, ok := v.(string)
			if !ok {
				return p, errors.New("ICE 凭据必须是字符串")
			}
			if *dest != "" {
				return p, errors.New("ICE 凭据重复")
			}
			*dest = s
		}
	}
	return finish(p)
}
