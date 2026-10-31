package devid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type ReportRequest struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type Reporter func(context.Context, ReportRequest) (int, []byte, error)

// ValidateReport restricts captured SDK traffic to the configured SDK service;
// browser metadata is never allowed to choose an arbitrary network target.
func ValidateReport(report ReportRequest) error {
	u, err := url.Parse(report.URL)
	if err != nil || u.Scheme != "https" || u.Host != "fp-it-acc.portal101.cn" || u.Path != "/deviceprofile/v4" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("devid: unexpected SDK report endpoint")
	}
	if len(report.Body) == 0 || len(report.Body) > 2<<20 || !json.Valid([]byte(report.Body)) {
		return fmt.Errorf("devid: invalid SDK report payload")
	}
	return nil
}

func deviceIDFromReport(status int, body []byte) (string, error) {
	var result struct {
		Code   int `json:"code"`
		Detail struct {
			DeviceID string `json:"deviceId"`
		} `json:"detail"`
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("devid: SDK service returned HTTP %d", status)
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("devid: invalid SDK service JSON: %w", err)
	}
	id := strings.TrimSpace(result.Detail.DeviceID)
	if result.Code != 1100 || id == "" {
		return "", fmt.Errorf("devid: SDK service rejected report (code=%d)", result.Code)
	}
	if !strings.HasPrefix(id, "B") {
		id = "B" + id
	}
	return id, nil
}
