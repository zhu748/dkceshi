package promptcompat

import (
	"crypto/sha256"
	"encoding/hex"
)

type ContextDigest struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type ContextMessageDigest struct {
	Index int    `json:"index"`
	Role  string `json:"role"`
	ContextDigest
	PromptContent ContextDigest `json:"prompt_content"`
	// NormalizedMessage fingerprints the complete renderer input, including
	// identity and extension context beside the body. It is not a coverage claim.
	NormalizedMessage ContextDigest        `json:"normalized_message"`
	OmittedReason     string               `json:"omitted_reason,omitempty"`
	OmittedPaths      []string             `json:"omitted_paths,omitempty"`
	Fields            []ContextFieldDigest `json:"fields,omitempty"`
}

// ContextManifest describes normalized API context and its final transport.
// Digests enable comparisons without duplicating message/attachment contents.
// It does not assert that an upstream model read or understood the text.
type ContextManifest struct {
	Scope           string                 `json:"scope"`
	RawRequest      *ContextDigest         `json:"raw_request,omitempty"`
	InputFields     []ContextFieldDigest   `json:"input_fields,omitempty"`
	SourceMessages  []ContextMessageDigest `json:"source_messages"`
	LiveMessages    []ContextMessageDigest `json:"live_messages"`
	HistoryMessages []ContextMessageDigest `json:"history_messages,omitempty"`
	Prompt          ContextDigest          `json:"final_prompt"`
	History         ContextDigest          `json:"history_text"`
	Tools           ContextDigest          `json:"tool_catalog"`
	RefFileIDs      []string               `json:"ref_file_ids,omitempty"`
	Externalized    bool                   `json:"externalized"`
	Omissions       []ContextOmission      `json:"omissions,omitempty"`
}

func digestContext(data []byte) ContextDigest {
	sum := sha256.Sum256(data)
	return ContextDigest{Bytes: len(data), SHA256: hex.EncodeToString(sum[:])}
}

func (r StandardRequest) ContextMessageDigests() []ContextMessageDigest {
	out := make([]ContextMessageDigest, 0, len(r.Messages))
	normalized := NormalizeLegacyToolHistory(r.Messages)
	for i, raw := range r.Messages {
		sourceDigest, err := digestJSONContext(raw)
		if err != nil {
			continue
		}
		message, _ := raw.(map[string]any)
		digest := ContextMessageDigest{Index: i, Role: asString(message["role"]), ContextDigest: sourceDigest}
		visible := NormalizeOpenAIMessagesForPrompt([]any{normalized[i]}, "", r.ToolMarker)
		digest.Fields = contextFieldDigests(message, len(visible) > 0)
		digest.OmittedPaths = contentOmittedPaths(message)
		if len(visible) > 0 {
			digest.PromptContent = digestContext([]byte(asString(visible[0]["content"])))
			digest.NormalizedMessage, _ = digestJSONContext(visible[0])
		} else if message == nil {
			digest.OmittedReason = "not_a_message_object"
		} else {
			digest.OmittedReason = "empty_prompt_message"
		}
		out = append(out, digest)
	}
	return out
}

func (r StandardRequest) ContextManifest() ContextManifest {
	live := r.ContextMessageDigests()
	source := cloneContextDigests(r.SourceContext)
	if source == nil {
		source = live
	}
	tools, _ := digestJSONContext(r.ToolsRaw)
	var raw *ContextDigest
	if r.RawRequest != nil {
		copy := *r.RawRequest
		raw = &copy
	}
	return ContextManifest{Scope: "normalized_api_context", SourceMessages: source, LiveMessages: live,
		HistoryMessages: cloneContextDigests(r.HistoryContext),
		RawRequest:      raw,
		InputFields:     append([]ContextFieldDigest(nil), r.InputContext...),
		Prompt:          digestContext([]byte(r.FinalPrompt)), History: digestContext([]byte(r.HistoryText)),
		Tools: tools, RefFileIDs: append([]string(nil), r.RefFileIDs...), Externalized: r.CurrentInputFileApplied, Omissions: contextOmissions(source)}
}
