package chathistory

import (
	"encoding/json"
	"errors"
)

const (
	FileVersion      = 2
	DisabledLimit    = 0
	DefaultLimit     = 20
	MaxLimit         = 50
	defaultPreviewAt = 160
)

var allowedLimits = map[int]struct{}{
	DisabledLimit: {},
	10:            {},
	20:            {},
	50:            {},
}

var ErrDisabled = errors.New("chat history disabled")

type Entry struct {
	ID               string          `json:"id"`
	Revision         int64           `json:"revision"`
	CreatedAt        int64           `json:"created_at"`
	UpdatedAt        int64           `json:"updated_at"`
	CompletedAt      int64           `json:"completed_at,omitempty"`
	Status           string          `json:"status"`
	CallerID         string          `json:"caller_id,omitempty"`
	UserAgent        string          `json:"user_agent,omitempty"`
	AccountID        string          `json:"account_id,omitempty"`
	Surface          string          `json:"surface,omitempty"`
	Model            string          `json:"model,omitempty"`
	Stream           bool            `json:"stream"`
	UserInput        string          `json:"user_input,omitempty"`
	Messages         []Message       `json:"messages,omitempty"`
	Tools            json.RawMessage `json:"tools,omitempty"`
	ToolChoice       json.RawMessage `json:"tool_choice_policy,omitempty"`
	ToolMarker       string          `json:"tool_marker,omitempty"`
	HistoryText      string          `json:"history_text,omitempty"`
	FinalPrompt      string          `json:"final_prompt,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Content          string          `json:"content,omitempty"`
	Error            string          `json:"error,omitempty"`
	StatusCode       int             `json:"status_code,omitempty"`
	ElapsedMs        int64           `json:"elapsed_ms,omitempty"`
	FinishReason     string          `json:"finish_reason,omitempty"`
	Usage            map[string]any  `json:"usage,omitempty"`
}

type Message struct {
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	Name             string          `json:"name,omitempty"`
	ID               string          `json:"id,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
	FunctionCall     json.RawMessage `json:"function_call,omitempty"`
	ReasoningContent json.RawMessage `json:"reasoning_content,omitempty"`
	IsError          *bool           `json:"is_error,omitempty"`
	Prefix           *bool           `json:"prefix,omitempty"`
}

type SummaryEntry struct {
	ID             string `json:"id"`
	Revision       int64  `json:"revision"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	CompletedAt    int64  `json:"completed_at,omitempty"`
	Status         string `json:"status"`
	CallerID       string `json:"caller_id,omitempty"`
	AccountID      string `json:"account_id,omitempty"`
	Surface        string `json:"surface,omitempty"`
	Model          string `json:"model,omitempty"`
	Stream         bool   `json:"stream"`
	UserInput      string `json:"user_input,omitempty"`
	Preview        string `json:"preview,omitempty"`
	StatusCode     int    `json:"status_code,omitempty"`
	ElapsedMs      int64  `json:"elapsed_ms,omitempty"`
	FinishReason   string `json:"finish_reason,omitempty"`
	DetailRevision int64  `json:"detail_revision"`
}

type File struct {
	Version  int            `json:"version"`
	Limit    int            `json:"limit"`
	Revision int64          `json:"revision"`
	Items    []SummaryEntry `json:"items"`
}

type StartParams struct {
	CallerID    string
	UserAgent   string
	AccountID   string
	Surface     string
	Model       string
	Stream      bool
	UserInput   string
	Messages    []Message
	Tools       json.RawMessage
	ToolChoice  json.RawMessage
	ToolMarker  string
	HistoryText string
	FinalPrompt string
}

type UpdateParams struct {
	Status           string
	ReasoningContent string
	Content          string
	Error            string
	StatusCode       int
	ElapsedMs        int64
	FinishReason     string
	Usage            map[string]any
	Completed        bool
}

type detailEnvelope struct {
	Version int   `json:"version"`
	Item    Entry `json:"item"`
}

func isAllowedLimit(limit int) bool {
	_, ok := allowedLimits[limit]
	return ok
}
