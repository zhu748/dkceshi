// Package tooldiagnostics records tool-output processing without owning parsing
// or any client protocol. Diagnostics never change a completion's outcome.
package tooldiagnostics

import (
	"strings"
	"unicode/utf8"
)

const MaxTextBytes = 256 << 10

type Text struct {
	Text      string `json:"text"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
}

func Capture(value string) Text {
	out := Text{Text: value, Bytes: len(value)}
	if len(value) <= MaxTextBytes {
		return out
	}
	head, tail := MaxTextBytes/2, len(value)-MaxTextBytes/2
	for head > 0 && !utf8.RuneStart(value[head]) {
		head--
	}
	for tail < len(value) && !utf8.RuneStart(value[tail]) {
		tail++
	}
	out.Text = value[:head] + "\n…[truncated]…\n" + value[tail:]
	out.Truncated = true
	return out
}

type Step struct {
	HistoryID string `json:"history_id,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Stage     string `json:"stage"`
	Result    string `json:"result"`
	Reason    string `json:"reason,omitempty"`
	Error     string `json:"error,omitempty"`
	At        int64  `json:"at"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
	Input     Text   `json:"input,omitempty"`
	Output    Text   `json:"output,omitempty"`
}

type Call struct {
	Ordinal   int    `json:"ordinal"`
	Name      string `json:"name"`
	Parse     string `json:"parse"`
	Schema    string `json:"schema"`
	Error     string `json:"error,omitempty"`
	Arguments Text   `json:"arguments"`
	Delivery  string `json:"delivery"`
}

type Attempt struct {
	Number        int    `json:"number"`
	Kind          string `json:"kind"`
	AccountID     string `json:"account_id,omitempty"`
	StartedAt     int64  `json:"started_at"`
	CompletedAt   int64  `json:"completed_at,omitempty"`
	Method        string `json:"method,omitempty"`
	Result        string `json:"result"`
	ErrorCode     string `json:"error_code,omitempty"`
	Error         string `json:"error,omitempty"`
	Source        string `json:"source,omitempty"`
	Raw           Text   `json:"raw"`
	Residual      Text   `json:"residual"`
	Calls         []Call `json:"calls"`
	Steps         []Step `json:"steps"`
	RetryDecision string `json:"retry_decision,omitempty"`
	RetryReason   string `json:"retry_reason,omitempty"`
}

type Summary struct {
	Revision     int64    `json:"revision"`
	ID           string   `json:"id"`
	CreatedAt    int64    `json:"created_at"`
	UpdatedAt    int64    `json:"updated_at"`
	CompletedAt  int64    `json:"completed_at,omitempty"`
	Surface      string   `json:"surface"`
	Model        string   `json:"model,omitempty"`
	Stream       bool     `json:"stream"`
	Status       string   `json:"status"`
	Pinned       bool     `json:"pinned"`
	ToolNames    []string `json:"tool_names"`
	AttemptCount int      `json:"attempt_count"`
	ErrorCode    string   `json:"error_code,omitempty"`
	Error        string   `json:"error,omitempty"`
	Delivery     string   `json:"delivery"`
}

type Record struct {
	Summary
	EvidenceTruncated bool      `json:"evidence_truncated,omitempty"`
	CallerID          string    `json:"caller_id,omitempty"`
	Tools             Text      `json:"tools"`
	Policy            Text      `json:"policy"`
	Attempts          []Attempt `json:"attempts"`
}

type Settings struct {
	Repair  *RepairSettings `json:"repair,omitempty"`
	Enabled bool            `json:"enabled"`
	Limit   int             `json:"limit"`
}

type Index struct {
	Version  int       `json:"version"`
	Settings Settings  `json:"settings"`
	Revision int64     `json:"revision"`
	Items    []Summary `json:"items"`
}

type Query struct {
	Page, PageSize       int
	Status, Tool, Search string
	Pinned               bool
}
type Page struct {
	Items        []Summary `json:"items"`
	Total        int       `json:"total"`
	PinnedCount  int       `json:"pinned_count"`
	Page         int       `json:"page"`
	PageSize     int       `json:"page_size"`
	Settings     Settings  `json:"settings"`
	Revision     int64     `json:"revision"`
	StorageError string    `json:"storage_error,omitempty"`
}

func matches(item Summary, q Query) bool {
	if q.Pinned && !item.Pinned || q.Status != "" && q.Status != item.Status {
		return false
	}
	if q.Tool != "" {
		found := false
		for _, name := range item.ToolNames {
			if name == q.Tool {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return q.Search == "" || strings.Contains(strings.ToLower(item.ID+" "+item.Model+" "+item.Error+" "+strings.Join(item.ToolNames, " ")), strings.ToLower(q.Search))
}
