package translatorcliproxy

import (
	"bytes"
	"encoding/json"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"strings"
)

type openAIUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  *int
	Metadata         any
}

func extractOpenAIUsage(line []byte) (openAIUsage, bool) {
	raw := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(line), []byte("data:")))
	return extractOpenAIUsageFromJSON(raw)
}

// This is only a wire mapping; the source request has already been resolved.
func translatedUsage(target sdktranslator.Format, usage openAIUsage) map[string]any {
	var out map[string]any
	if target == sdktranslator.FormatClaude {
		out = map[string]any{"input_tokens": usage.PromptTokens, "output_tokens": usage.CompletionTokens}
	} else {
		out = map[string]any{"promptTokenCount": usage.PromptTokens, "candidatesTokenCount": usage.CompletionTokens, "totalTokenCount": usage.TotalTokens}
		if usage.ReasoningTokens != nil && *usage.ReasoningTokens <= usage.CompletionTokens {
			out["candidatesTokenCount"] = usage.CompletionTokens - *usage.ReasoningTokens
			out["thoughtsTokenCount"] = *usage.ReasoningTokens
		}
	}
	if metadata, ok := usage.Metadata.(map[string]any); ok && metadata["reasoning_source"] == "unknown" && target == sdktranslator.FormatGemini {
		delete(out, "candidatesTokenCount")
	}
	if usage.Metadata != nil {
		out["ds2api"] = usage.Metadata
	}
	return out
}

func injectStreamUsageMetadata(chunk []byte, target sdktranslator.Format, usage openAIUsage) []byte {
	if target != sdktranslator.FormatClaude && target != sdktranslator.FormatGemini {
		return chunk
	}
	lines := strings.Split(string(chunk), "\n")
	for i, line := range lines {
		text := strings.TrimSpace(line)
		prefix := ""
		if strings.HasPrefix(text, "data:") {
			prefix = "data: "
			text = strings.TrimSpace(strings.TrimPrefix(text, "data:"))
		}
		var obj map[string]any
		if json.Unmarshal([]byte(text), &obj) != nil || obj == nil {
			continue
		}
		if target == sdktranslator.FormatClaude {
			if obj["type"] != "message_delta" {
				continue
			}
			obj["usage"] = translatedUsage(target, usage)
		} else {
			if _, ok := obj["candidates"]; !ok {
				continue
			}
			obj["usageMetadata"] = translatedUsage(target, usage)
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			continue
		}
		lines[i] = prefix + string(raw)
	}
	return []byte(strings.Join(lines, "\n"))
}
