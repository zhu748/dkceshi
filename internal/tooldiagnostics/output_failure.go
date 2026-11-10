package tooldiagnostics

import "strings"

func (r *Recorder) hasToolOutputFailureLocked(code string) bool {
	switch code {
	case "upstream_unavailable", "upstream_empty_output", "rate_limit_exceeded", "account_unauthorized", "account_muted", "account_banned", "captcha_required":
		tools := strings.TrimSpace(r.record.Tools.Text)
		return tools != "" && tools != "null" && tools != "[]"
	}
	return false
}
