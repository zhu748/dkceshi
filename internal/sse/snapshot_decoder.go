package sse

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Track upstream fields, not tool values: identical APPEND calls are distinct.
// Resolve fragment IDs/indexes before thinking recovery changes their channels.
type snapshotDecoder struct {
	fields          map[string]*strings.Builder
	aliases         map[string]string
	last            map[string]string
	nextFragment    int
	cursor          string
	thinkingEnabled bool
	err             error
}

func (d *snapshotDecoder) normalize(chunk map[string]any, kind string, thinkingEnabled bool) map[string]any {
	d.cursor, d.thinkingEnabled = kind, thinkingEnabled
	first := len(d.fields) == 0
	if d.fields == nil {
		d.fields = map[string]*strings.Builder{}
		d.aliases = map[string]string{}
		d.last = map[string]string{}
	}
	out := d.patch(chunk, "", kind, false)
	// Initial replay envelopes still need overlap handling against an earlier
	// transport attempt. All later snapshots are already decoded to deltas.
	if !first || !isContentSnapshot(chunk) {
		out["o"] = "APPEND"
	}
	return out
}

func replayPath(parent, child string) string {
	child = strings.TrimPrefix(child, "/")
	if strings.HasPrefix(child, "response/") || child == "response" {
		return child
	}
	if parent == "" {
		return "response/" + child
	}
	return strings.TrimSuffix(parent, "/") + "/" + child
}

func (d *snapshotDecoder) patch(node map[string]any, parent, kind string, inherited bool) map[string]any {
	out := make(map[string]any, len(node))
	for k, v := range node {
		out[k] = v
	}
	path, _ := node["p"].(string)
	full := replayPath(parent, path)
	if shouldSkipPath(path) || isStatusPath(path) {
		return out
	}
	op, _ := node["o"].(string)
	snapshot := inherited || strings.EqualFold(op, "SET") || isContentSnapshot(node)
	if strings.EqualFold(op, "APPEND") {
		snapshot = false
	}
	if strings.Contains(full, "thinking") {
		kind = "thinking"
		if !bodyStarted(d.thinkingEnabled, d.cursor) {
			d.cursor = kind
		}
	}
	if isFragmentScopedPath(parent) && (path == "content" || path == "response/content") {
		kind = "thinking"
	} else if path == "response/content" {
		kind, d.cursor = "text", "text"
	}
	switch value := node["v"].(type) {
	case string:
		key := d.contentKey(full, kind)
		out["v"] = d.delta(key, kind, value, snapshot)
	case []any:
		out["v"] = d.array(value, full, snapshot)
	case map[string]any:
		out["v"] = d.object(value, full, kind, snapshot)
	}
	return out
}

func (d *snapshotDecoder) contentKey(path, kind string) string {
	if key := d.aliases[path]; key != "" {
		return key
	}
	if strings.Contains(path, "/fragments/") {
		segments := strings.Split(path, "/")
		for i, segment := range segments {
			if segment != "fragments" || i+1 >= len(segments) {
				continue
			}
			if index, err := strconv.Atoi(segments[i+1]); err == nil && index < 0 {
				alias := "response/fragments/" + strconv.Itoa(d.nextFragment+index) + "/content"
				if key := d.aliases[alias]; key != "" {
					return key
				}
				if d.last[kind] != "" {
					return d.last[kind]
				}
			}
		}
		return path
	}
	if d.last[kind] != "" {
		return d.last[kind]
	}
	return "channel:" + kind
}

func (d *snapshotDecoder) delta(key, kind, text string, snapshot bool) string {
	if text == "" {
		return ""
	}
	previous := d.fields[key]
	// Some streams begin with pathless deltas, then introduce their first
	// fragment ID in a snapshot. Bind that ID to the existing channel once.
	if previous == nil && snapshot {
		if old := d.last[kind]; strings.HasPrefix(old, "channel:") {
			previous = d.fields[old]
		}
	}
	if previous == nil {
		previous = &strings.Builder{}
	}
	d.fields[key], d.last[kind] = previous, key
	if snapshot {
		old := previous.String()
		if strings.HasPrefix(text, old) {
			text = text[len(old):]
		} else if strings.HasPrefix(old, text) {
			return ""
		} else {
			d.err = fmt.Errorf("upstream snapshot rewrote an already delivered content field")
			return ""
		}
	}
	previous.WriteString(text)
	return text
}

func (d *snapshotDecoder) array(values []any, path string, snapshot bool) []any {
	out := make([]any, len(values))
	base := 0
	fragments := strings.HasSuffix(path, "/fragments")
	if fragments && !snapshot {
		base = d.nextFragment
	}
	for i, value := range values {
		kind := d.cursor
		node, ok := value.(map[string]any)
		if !ok {
			out[i] = value
			continue
		}
		if _, typed := node["type"]; typed && node["content"] != nil {
			out[i] = d.fragment(node, base+i, kind, snapshot)
		} else {
			out[i] = d.patch(node, path, kind, snapshot)
		}
	}
	if fragments && base+len(values) > d.nextFragment {
		d.nextFragment = base + len(values)
	}
	return out
}

func (d *snapshotDecoder) object(value map[string]any, path, kind string, snapshot bool) map[string]any {
	out := make(map[string]any, len(value))
	for k, v := range value {
		out[k] = v
	}
	if response, ok := value["response"].(map[string]any); ok {
		out["response"] = d.object(response, "response", kind, snapshot)
	} else if fragments, ok := value["fragments"].([]any); ok {
		out["fragments"] = d.array(fragments, "response/fragments", snapshot)
	} else {
		for _, field := range []string{"text", "content"} {
			if text, ok := value[field].(string); ok {
				out[field] = d.delta(d.contentKey(path, kind), kind, text, snapshot)
				break
			}
		}
	}
	return out
}

func (d *snapshotDecoder) fragment(node map[string]any, index int, kind string, snapshot bool) map[string]any {
	out := make(map[string]any, len(node))
	for k, v := range node {
		out[k] = v
	}
	typ, _ := node["type"].(string)
	switch strings.ToUpper(typ) {
	case "THINK", "THINKING":
		kind = "thinking"
		if !bodyStarted(d.thinkingEnabled, d.cursor) {
			d.cursor = kind
		}
	case "RESPONSE":
		kind, d.cursor = "text", "text"
	default:
		if isToolFragmentType(strings.ToUpper(typ)) {
			kind = "thinking"
		}
	}
	alias := "response/fragments/" + strconv.Itoa(index) + "/content"
	key := alias
	if id, ok := node["id"]; ok {
		switch id.(type) {
		case string, float64, int, json.Number:
			if encoded, err := json.Marshal(id); err == nil {
				key = "fragment:" + string(encoded)
			}
		}
	}
	d.aliases[alias] = key
	if text, ok := node["content"].(string); ok {
		out["content"] = d.delta(key, kind, text, snapshot)
	}
	return out
}

// A replay envelope describes a whole channel, potentially in several parts.
// Join adjacent snapshot parts before the cross-transport overlap fallback.
func coalesceSnapshotParts(parts []ContentPart) []ContentPart {
	out := make([]ContentPart, 0, len(parts))
	for i := 0; i < len(parts); {
		part := parts[i]
		i++
		if part.Snapshot {
			var text strings.Builder
			text.WriteString(part.Text)
			for i < len(parts) && parts[i].Snapshot && parts[i].Type == part.Type {
				text.WriteString(parts[i].Text)
				i++
			}
			part.Text = text.String()
		}
		out = append(out, part)
	}
	return out
}
