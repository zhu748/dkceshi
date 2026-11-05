package proxyinput

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

func collectCandidates(text string) ([]candidate, error) {
	items := []candidate{}
	var expand func(any, string, int) error
	expand = func(value any, source string, depth int) error {
		if depth > 24 {
			return errors.New("JSON 嵌套过深")
		}
		if a, ok := value.([]any); ok {
			for i, v := range a {
				if err := expand(v, fmt.Sprintf("%s[%d]", source, i+1), depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		if m, ok := value.(map[string]any); ok {
			if ice, exists := m["iceServers"]; exists && len(m) == 1 {
				if a, ok := ice.([]any); ok {
					return expand(a, source+".iceServers", depth+1)
				}
			}
			if urls, ok := m["urls"].([]any); ok && len(urls) > 0 {
				for i, u := range urls {
					copy := map[string]any{}
					for k, v := range m {
						copy[k] = v
					}
					copy["urls"] = u
					if err := expand(copy, fmt.Sprintf("%s.urls[%d]", source, i+1), depth+1); err != nil {
						return err
					}
				}
				return nil
			}
		}
		if len(items) >= MaxItems {
			return errors.New("每次最多识别 200 条节点")
		}
		items = append(items, candidate{source: source, value: value})
		return nil
	}
	trimmed := strings.TrimSpace(text)
	if looksJSON(trimmed) {
		value, err := DecodeJSON(trimmed)
		if err == nil {
			if err = expand(value, "JSON", 0); err != nil {
				return nil, err
			}
			if len(items) == 0 {
				return nil, errors.New("JSON 没有可识别的代理节点")
			}
			return items, nil
		}
		// A standalone valid node on another line identifies a line-based import.
		// Keep malformed pretty-printed JSON as one error instead of treating fields as nodes.
		lineBased := false
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if looksJSON(line) {
				if _, e := DecodeJSON(line); e == nil {
					lineBased = true
					break
				}
			} else if _, e := Parse(line, "turn"); e == nil {
				lineBased = true
				break
			}
		}
		if !lineBased {
			return nil, err
		}
	}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(line, "|") {
			line = strings.TrimSpace(line)
		}
		var value any = line
		if looksJSON(line) {
			if decoded, e := DecodeJSON(line); e == nil {
				value = decoded
			}
		}
		if err := expand(value, fmt.Sprintf("第 %d 行", i+1), 0); err != nil {
			return nil, err
		}
	}
	if len(items) == 0 {
		return nil, errors.New("没有识别到代理节点")
	}
	return items, nil
}
func looksJSON(value string) bool {
	if strings.HasPrefix(value, "{") {
		return true
	}
	if !strings.HasPrefix(value, "[") {
		return false
	}
	if end := strings.Index(value, "]"); end > 0 && net.ParseIP(value[1:end]) != nil {
		return false
	}
	return true
}
