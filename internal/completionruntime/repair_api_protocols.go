package completionruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ds2api/internal/tooldiagnostics"
)

func buildRepairAPIPayload(settings tooldiagnostics.RepairSettings, prompt string) (map[string]any, error) {
	protocol := tooldiagnostics.RepairProtocol(settings.Protocol)
	payload := map[string]any{"model": settings.Model, "messages": []any{map[string]any{"role": "user", "content": prompt}}, "stream": false}
	switch protocol {
	case "claude":
		payload["max_tokens"] = 8192
	case "gemini":
		payload = map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": prompt}}}}, "generationConfig": map[string]any{"maxOutputTokens": 8192, "candidateCount": 1}}
	}
	if len(settings.ExtraBody) == 0 {
		return payload, nil
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(settings.ExtraBody, &extra); err != nil {
		return nil, err
	}
	for key, value := range extra {
		if protocol == "gemini" && (key == "generationConfig" || key == "generation_config") {
			var generation map[string]json.RawMessage
			if err := json.Unmarshal(value, &generation); err != nil {
				return nil, err
			}
			config := payload["generationConfig"].(map[string]any)
			for name, v := range generation {
				config[geminiGenerationKey(name)] = v
			}
			continue
		}
		payload[key] = value
	}
	return payload, nil
}

func parseClaudeRepairOutput(data []byte) (repairOutput, error) {
	var response struct {
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return repairOutput{}, errors.New("claude repair API returned invalid response")
	}
	if response.StopReason != "" && response.StopReason != "end_turn" && response.StopReason != "stop_sequence" {
		return repairOutput{}, fmt.Errorf("claude repair API did not complete normally: %s", response.StopReason)
	}
	var text, thinking strings.Builder
	for _, block := range response.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "thinking":
			thinking.WriteString(block.Thinking)
		case "redacted_thinking":
			continue
		default:
			return repairOutput{}, errors.New("claude repair API returned non-text output")
		}
	}
	return repairOutput{Text: text.String(), Thinking: thinking.String()}, nil
}

func parseGeminiRepairOutput(data []byte) (repairOutput, error) {
	var response struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text    *string `json:"text"`
					Thought bool    `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return repairOutput{}, errors.New("gemini repair API returned invalid response")
	}
	if response.PromptFeedback.BlockReason != "" {
		return repairOutput{}, errors.New("gemini repair API blocked the prompt")
	}
	if len(response.Candidates) != 1 {
		return repairOutput{}, errors.New("gemini repair API must return one candidate")
	}
	candidate := response.Candidates[0]
	if candidate.FinishReason != "" && candidate.FinishReason != "STOP" {
		return repairOutput{}, fmt.Errorf("gemini repair API did not complete normally: %s", candidate.FinishReason)
	}
	var text, thinking strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Text == nil {
			return repairOutput{}, errors.New("gemini repair API returned non-text output")
		}
		if part.Thought {
			thinking.WriteString(*part.Text)
		} else {
			text.WriteString(*part.Text)
		}
	}
	return repairOutput{Text: text.String(), Thinking: thinking.String()}, nil
}

func geminiGenerationKey(key string) string {
	parts := strings.Split(key, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}
