package openai

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/jsonvalue"
	"ds2api/internal/toolcall"
	"ds2api/internal/toolstream"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

func BuildResponseObject(responseID, model, finalPrompt, finalThinking, finalText string, toolNames []string, toolsRaw any) map[string]any {
	// Strict mode: only standalone, structured tool-call payloads are treated
	// as executable tool calls.
	detected := toolcall.ParseAssistantToolCallsDetailed(finalText, finalThinking, toolNames)
	return BuildResponseObjectWithToolCalls(responseID, model, finalPrompt, finalThinking, finalText, detected.Calls, toolsRaw)
}

func BuildResponseObjectWithToolCalls(responseID, model, finalPrompt, finalThinking, finalText string, detected []toolcall.ParsedToolCall, toolsRaw any) map[string]any {
	return buildResponseObject(responseID, model, finalPrompt, finalThinking, finalText, toResponsesFunctionCallItems(detected, toolsRaw))
}

func BuildResponseObjectFromTurn(responseID, model string, turn assistantturn.Turn, toolsRaw any) map[string]any {
	return buildResponseObject(responseID, model, turn.Prompt, turn.Thinking, turn.Text, toValidatedResponsesFunctionCallItems(turn.ToolCalls, toolsRaw))
}

func buildResponseObject(responseID, model, finalPrompt, finalThinking, finalText string, calls []any) map[string]any {
	if len(calls) > 0 {
		finalText = toolstream.ToolResponseText(finalText)
	}
	exposedOutputText := finalText
	output := make([]any, 0, 3)
	if strings.TrimSpace(finalThinking) != "" {
		output = append(output, buildResponsesReasoningItem(finalThinking))
	}
	if strings.TrimSpace(finalText) != "" {
		output = append(output, map[string]any{
			"type": "message", "id": "msg_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"role": "assistant", "status": "completed",
			"content": []any{map[string]any{"type": "output_text", "text": finalText}},
		})
	}
	if len(calls) > 0 {
		output = append(output, calls...)
	} else if strings.TrimSpace(finalText) == "" && strings.TrimSpace(finalThinking) != "" {
		exposedOutputText = finalThinking
	}
	return BuildResponseObjectFromItems(
		responseID,
		model,
		finalPrompt,
		finalThinking,
		finalText,
		output,
		exposedOutputText,
	)
}

func buildResponsesReasoningItem(finalThinking string) map[string]any {
	return map[string]any{
		"id":   "rsn_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		"type": "reasoning",
		"summary": []any{map[string]any{
			"type": "summary_text",
			"text": finalThinking,
		}},
		"content": []any{map[string]any{
			"type": "reasoning_text",
			"text": finalThinking,
		}},
		"status": "completed",
	}
}

func BuildResponseObjectFromItems(responseID, model, finalPrompt, finalThinking, finalText string, output []any, outputText string) map[string]any {
	if output == nil {
		output = []any{}
	}
	return map[string]any{
		"id":          responseID,
		"type":        "response",
		"object":      "response",
		"created_at":  time.Now().Unix(),
		"status":      "completed",
		"model":       model,
		"output":      output,
		"output_text": outputText,
		"usage":       BuildResponsesUsageForModel(model, finalPrompt, finalThinking, finalText, 0),
	}
}

func toResponsesFunctionCallItems(toolCalls []toolcall.ParsedToolCall, toolsRaw any) []any {
	return toValidatedResponsesFunctionCallItems(toolcall.NormalizeParsedToolCallsForSchemas(toolCalls, toolsRaw), toolsRaw)
}

func toValidatedResponsesFunctionCallItems(toolCalls []toolcall.ParsedToolCall, toolsRaw any) []any {
	if len(toolCalls) == 0 {
		return nil
	}
	out := make([]any, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if strings.TrimSpace(tc.Name) == "" {
			continue
		}
		field, value := ResponsesToolInput(tc.Name, tc.Input, toolsRaw)
		prefix := "fc_"
		if field == "input" {
			prefix = "ctc_"
		}
		out = append(out, BuildResponsesToolItem(tc.Name, field, value, prefix+strings.ReplaceAll(uuid.NewString(), "-", ""), "call_"+strings.ReplaceAll(uuid.NewString(), "-", ""), "completed"))
	}
	return out
}

func normalizeJSONString(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "{}"
	}
	var v any
	if err := jsonvalue.Unmarshal([]byte(s), &v); err != nil {
		return raw
	}
	b, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return string(b)
}
