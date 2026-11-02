package completionruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"ds2api/internal/config"
	"ds2api/internal/streambudget"
	"ds2api/internal/tooldiagnostics"
)

// Shared transport for generation and discovery: bounded reads, no redirects,
// caller cancellation and protocol-specific credentials kept in headers.
func doRepairAPIRequest(ctx context.Context, settings tooldiagnostics.RepairSettings, method, endpoint string, payload any, limit int) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	switch tooldiagnostics.RepairProtocol(settings.Protocol) {
	case "claude":
		req.Header.Set("anthropic-version", "2023-06-01")
		if settings.APIKey != "" {
			req.Header.Set("x-api-key", settings.APIKey)
		}
	case "gemini":
		if settings.APIKey != "" {
			req.Header.Set("x-goog-api-key", settings.APIKey)
		}
	default:
		if settings.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+settings.APIKey)
		}
	}
	client := &http.Client{Transport: http.DefaultTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, scrubRepairError(err, settings.APIKey)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			config.Logger.Warn("[toolcall_repair] close custom response failed", "error", scrubRepairError(err, settings.APIKey))
		}
	}()
	if limit <= 0 {
		// Repair is a small isolated format operation. Bound the JSON envelope
		// (including thinking) independently of the main completion's output.
		limit = min(streambudget.Value("max_output_bytes"), 4*streambudget.Value("max_repair_bytes"))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return nil, scrubRepairError(err, settings.APIKey)
	}
	if len(data) > limit {
		return nil, errors.New("custom repair API response exceeds output limit")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, customRepairResponseError(resp.StatusCode, data, settings.APIKey)
	}
	if !json.Valid(data) {
		return nil, errors.New("custom repair API returned invalid JSON")
	}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil && len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return nil, customRepairResponseError(http.StatusBadGateway, data, settings.APIKey)
	}
	return data, nil
}
