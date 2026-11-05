package promptcompat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type ContextDigest struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type ContextMessageDigest struct {
	Index int    `json:"index"`
	Role  string `json:"role"`
	ContextDigest
}

// ContextManifest describes normalized API context and its final transport.
// Digests enable comparisons without duplicating message/attachment contents.
// It does not assert that an upstream model read or understood the text.
type ContextManifest struct {
	Scope          string                 `json:"scope"`
	SourceMessages []ContextMessageDigest `json:"source_messages"`
	LiveMessages   []ContextMessageDigest `json:"live_messages"`
	Prompt         ContextDigest          `json:"final_prompt"`
	History        ContextDigest          `json:"history_text"`
	Tools          ContextDigest          `json:"tool_catalog"`
	RefFileIDs     []string               `json:"ref_file_ids,omitempty"`
	Externalized   bool                   `json:"externalized"`
}

func digestContext(data []byte) ContextDigest {
	sum := sha256.Sum256(data)
	return ContextDigest{Bytes: len(data), SHA256: hex.EncodeToString(sum[:])}
}

func (r StandardRequest) ContextMessageDigests() []ContextMessageDigest {
	out := make([]ContextMessageDigest, 0, len(r.Messages))
	for i, raw := range r.Messages {
		data, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		message, _ := raw.(map[string]any)
		out = append(out, ContextMessageDigest{Index: i, Role: asString(message["role"]), ContextDigest: digestContext(data)})
	}
	return out
}

func (r StandardRequest) ContextManifest() ContextManifest {
	live := r.ContextMessageDigests()
	source := append([]ContextMessageDigest(nil), r.SourceContext...)
	if source == nil {
		source = live
	}
	tools, _ := json.Marshal(r.ToolsRaw)
	return ContextManifest{Scope: "normalized_api_context", SourceMessages: source, LiveMessages: live,
		Prompt: digestContext([]byte(r.FinalPrompt)), History: digestContext([]byte(r.HistoryText)),
		Tools: digestContext(tools), RefFileIDs: append([]string(nil), r.RefFileIDs...), Externalized: r.CurrentInputFileApplied}
}
