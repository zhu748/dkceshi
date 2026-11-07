package completionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/tooldiagnostics"
)

type repairAPIError struct {
	status     int
	message    string
	retryAfter time.Duration
}

func (e *repairAPIError) Error() string             { return e.message }
func (e *repairAPIError) RetryDelay() time.Duration { return e.retryAfter }
func (e *repairAPIError) Retryable() bool {
	return e.status == 408 || e.status == 409 || e.status == 425 || e.status == 429 || e.status >= 500
}

// A single isolated user message carries only the format-repair prompt. No
// original history, file IDs, tools catalog or upstream credentials are sent.
func callRepairAPI(ctx context.Context, settings tooldiagnostics.RepairSettings, prompt string) (repairOutput, error) {
	if err := pausedRepairConfiguration(ctx, settings); err != nil {
		return repairOutput{}, err
	}
	endpoint, err := tooldiagnostics.RepairAPIEndpoint(settings)
	if err != nil {
		return repairOutput{}, err
	}
	payload, err := buildRepairAPIPayload(settings, prompt)
	if err != nil {
		return repairOutput{}, &repairAPIError{status: http.StatusBadRequest, message: "invalid repair API parameters: " + err.Error()}
	}
	data, err := doRepairAPIRequest(ctx, settings, http.MethodPost, endpoint, payload, 0)
	if err != nil {
		rememberRepairConfigurationFailure(ctx, settings, err)
		return repairOutput{}, err
	}
	var output repairOutput
	switch tooldiagnostics.RepairProtocol(settings.Protocol) {
	case "claude":
		output, err = parseClaudeRepairOutput(data)
	case "gemini":
		output, err = parseGeminiRepairOutput(data)
	default:
		output, err = parseOpenAIRepairOutput(data, settings.APIKey)
	}
	if err != nil {
		return repairOutput{}, scrubRepairError(err, settings.APIKey)
	}
	clearRepairConfigurationFailure(settings)
	return output, nil
}

func parseOpenAIRepairOutput(data []byte, key string) (repairOutput, error) {
	var result struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content          json.RawMessage   `json:"content"`
				ReasoningContent string            `json:"reasoning_content"`
				Reasoning        string            `json:"reasoning"`
				ToolCalls        []json.RawMessage `json:"tool_calls"`
				FunctionCall     json.RawMessage   `json:"function_call"`
				Refusal          *string           `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return repairOutput{}, fmt.Errorf("custom repair API returned invalid JSON")
	}
	if result.Error != nil {
		return repairOutput{}, scrubRepairError(fmt.Errorf("custom repair API: %s", result.Error.Message), key)
	}
	if len(result.Choices) != 1 {
		return repairOutput{}, fmt.Errorf("custom repair API must return one completion choice")
	}
	if reason := result.Choices[0].FinishReason; reason != "" && reason != "stop" {
		return repairOutput{}, fmt.Errorf("custom repair API did not complete normally: %s", reason)
	}
	message := result.Choices[0].Message
	if len(message.ToolCalls) > 0 || len(message.FunctionCall) > 0 && string(message.FunctionCall) != "null" {
		return repairOutput{}, errors.New("custom repair API returned structured tool output")
	}
	if message.Refusal != nil && strings.TrimSpace(*message.Refusal) != "" {
		return repairOutput{}, errors.New("custom repair API refused the repair request")
	}
	text, err := repairAPIContent(message.Content)
	if err != nil {
		return repairOutput{}, err
	}
	thinking := message.ReasoningContent
	if thinking == "" {
		thinking = message.Reasoning
	}
	return repairOutput{Text: text, Thinking: thinking}, nil
}

func repairAPIContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return "", fmt.Errorf("custom repair API returned unsupported message content")
	}
	var out strings.Builder
	for _, part := range parts {
		if part.Type != "text" && part.Type != "output_text" {
			return "", fmt.Errorf("custom repair API returned non-text content")
		}
		out.WriteString(part.Text)
	}
	return out.String(), nil
}

func customRepairResponseError(status int, body []byte, key string) error {
	var result struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	message := "request failed"
	if json.Unmarshal(body, &result) == nil && result.Error.Message != "" {
		message = result.Error.Message
	}
	clean := scrubRepairError(fmt.Errorf("custom repair API HTTP %d: %s", status, message), key)
	return &repairAPIError{status: status, message: clean.Error()}
}

func scrubRepairError(err error, key string) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	message := err.Error()
	if key != "" {
		message = strings.ReplaceAll(message, key, "[redacted]")
	}
	runes := []rune(message)
	if len(runes) > 2048 {
		message = string(runes[:2048]) + "…"
	}
	return errors.New(message)
}
