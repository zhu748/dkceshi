package translatorcliproxy

import (
	"bytes"
	"context"
	"ds2api/internal/tokenusage"
	"encoding/json"
	"strings"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

func ToOpenAI(from sdktranslator.Format, model string, raw []byte, stream bool) []byte {
	return sdktranslator.TranslateRequest(from, sdktranslator.FormatOpenAI, model, raw, stream)
}

func FromOpenAINonStream(to sdktranslator.Format, model string, originalReq, translatedReq, raw []byte) []byte {
	var param any
	converted := sdktranslator.TranslateNonStream(context.Background(), sdktranslator.FormatOpenAI, to, model, originalReq, translatedReq, raw, &param)
	usage, ok := extractOpenAIUsageFromJSON(raw)
	if !ok {
		return converted
	}
	return injectNonStreamUsageMetadata(converted, to, usage)
}

func FromOpenAIStream(to sdktranslator.Format, model string, originalReq, translatedReq, streamBody []byte) []byte {
	var out bytes.Buffer
	var param any
	var latest *openAIUsage
	for _, line := range bytes.Split(streamBody, []byte("\n")) {
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			continue
		}
		payload := append([]byte(nil), line...)
		if !bytes.HasPrefix(payload, []byte("data:")) {
			continue
		}
		if usage, ok := extractOpenAIUsage(payload); ok {
			latest = &usage
		}
		chunks := sdktranslator.TranslateStream(context.Background(), sdktranslator.FormatOpenAI, to, model, originalReq, translatedReq, payload, &param)
		if latest != nil {
			for i := range chunks {
				chunks[i] = injectStreamUsageMetadata(chunks[i], to, *latest)
			}
		}
		for i := range chunks {
			out.Write(chunks[i])
			if !bytes.HasSuffix(chunks[i], []byte("\n")) {
				out.WriteByte('\n')
			}
		}
	}
	return out.Bytes()
}

func ParseFormat(name string) sdktranslator.Format {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "openai", "openai-chat", "chat", "chat-completions":
		return sdktranslator.FormatOpenAI
	case "openai-response", "responses", "openai-responses":
		return sdktranslator.FormatOpenAIResponse
	case "claude", "anthropic":
		return sdktranslator.FormatClaude
	case "gemini", "google":
		return sdktranslator.FormatGemini
	case "gemini-cli", "geminicli":
		return sdktranslator.FormatGeminiCLI
	case "codex", "openai-codex":
		return sdktranslator.FormatCodex
	case "antigravity":
		return sdktranslator.FormatAntigravity
	default:
		return sdktranslator.FromString(name)
	}
}

func ToOpenAIByName(formatName, model string, raw []byte, stream bool) []byte {
	return ToOpenAI(ParseFormat(formatName), model, raw, stream)
}

func extractOpenAIUsageFromJSON(raw []byte) (openAIUsage, bool) {
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return openAIUsage{}, false
	}
	usageObj, _ := payload["usage"].(map[string]any)
	if usageObj == nil {
		return openAIUsage{}, false
	}
	summary := tokenusage.Summarize(usageObj)
	if summary.PromptTokens == nil && summary.CompletionTokens == nil && summary.TotalTokens == nil {
		return openAIUsage{}, false
	}
	result := openAIUsage{ReasoningTokens: summary.ReasoningTokens, Metadata: tokenusage.CloneMap(usageObj)["ds2api"]}
	if summary.PromptTokens != nil {
		result.PromptTokens = *summary.PromptTokens
	}
	if summary.CompletionTokens != nil {
		result.CompletionTokens = *summary.CompletionTokens
	}
	result.TotalTokens, _ = tokenusage.Count(int64(result.PromptTokens) + int64(result.CompletionTokens))
	return result, true
}

func injectNonStreamUsageMetadata(converted []byte, target sdktranslator.Format, usage openAIUsage) []byte {
	obj := map[string]any{}
	if err := json.Unmarshal(converted, &obj); err != nil {
		return converted
	}
	switch target {
	case sdktranslator.FormatClaude:
		obj["usage"] = translatedUsage(target, usage)
	case sdktranslator.FormatGemini:
		obj["usageMetadata"] = translatedUsage(target, usage)
	default:
		return converted
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return converted
	}
	return out
}
