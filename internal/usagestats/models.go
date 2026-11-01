package usagestats

// Entry 表示某一天某个模型在某个调用方（API Key）和账号下的累计 Token 用量与费用。
type Entry struct {
	Date             string  `json:"date"`
	Model            string  `json:"model"`
	CallerID         string  `json:"caller_id,omitempty"`
	AccountID        string  `json:"account_id,omitempty"`
	AccountLabel     string  `json:"account_label,omitempty"`
	LastUsedAt       int64   `json:"last_used_at,omitempty"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Cost             float64 `json:"cost,omitempty"`
	Calls            int64   `json:"calls"`
}

// BackfillItem 是聊天历史回填的输入条目。
type BackfillItem struct {
	Timestamp  int64
	Model      string
	CallerID   string
	Prompt     int64
	Completion int64
	Reasoning  int64
	Total      int64
}
