package proxyinput

import (
	"encoding/json"
	"errors"
	"strconv"
)

func objectOptions(m map[string]any) (map[string]string, error) {
	opts := map[string]string{}
	seen := map[string]bool{}
	add := func(key string, value any) error {
		switch key {
		case "tls_server_name":
			key = "sni"
		case "ca_file":
			key = "ca"
		}
		if seen[key] {
			return errors.New("高级参数重复")
		}
		seen[key] = true
		switch key {
		case "sni", "ca", "realm", "transport":
			v, ok := value.(string)
			if !ok {
				return errors.New("高级文本参数必须为字符串")
			}
			opts[key] = v
		case "insecure":
			switch v := value.(type) {
			case bool:
				opts[key] = strconv.FormatBool(v)
			case string:
				opts[key] = v
			default:
				return errors.New("insecure 必须为布尔值或布尔字符串")
			}
		case "mtu":
			switch v := value.(type) {
			case json.Number:
				opts[key] = v.String()
			case string:
				opts[key] = v
			default:
				return errors.New("MTU 必须为整数")
			}
		default:
			return errors.New("节点 JSON 含未知高级参数")
		}
		return nil
	}
	if value, ok := m["options"]; ok {
		o, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("options 必须为对象")
		}
		for key, v := range o {
			if err := add(key, v); err != nil {
				return nil, err
			}
		}
	}
	for _, key := range []string{"tls_server_name", "ca_file", "insecure", "realm", "mtu"} {
		if v, ok := m[key]; ok {
			if err := add(key, v); err != nil {
				return nil, err
			}
		}
	}
	return opts, nil
}
