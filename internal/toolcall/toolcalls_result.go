package toolcall

type ParsedToolCall struct {
	NormalizationLimitExceeded bool `json:"-"`
	// CallID is retained for replaying text-form history. Response adapters
	// continue allocating their own delivery IDs; model IDs are not trusted.
	CallID         string            `json:"-"`
	RecoverySteps  []string          `json:"-"`
	Name           string            `json:"name"`
	Input          map[string]any    `json:"input"`
	StringLiterals map[string]string `json:"-"`
	// Repeated parameters become arrays; retain each occurrence's literal bytes.
	RepeatedStringLiterals map[string]map[int]string `json:"-"`
}

type ToolCallParseResult struct {
	ResourceLimitExceeded bool     `json:"-"`
	SourceChannel         string   `json:"-"`
	RecoverySteps         []string `json:"-"`
	Calls                 []ParsedToolCall
	SawToolCallSyntax     bool
	SawToolCallIntent     bool
	// ResidualIntentText carries the residual (pre-normalization, fences
	// stripped, successful-wrapper-deducted) text that triggered
	// SawToolCallIntent. It is the "bad tool-call code" the LLM repair layer
	// (plan/tool-call-fallback-design-phase3.md) operates on. It is only
	// populated when SawToolCallIntent is true.
	ResidualIntentText string
	// ResidualProtocolText also retains incomplete prefixes without a paired
	// intent signal, so final validation cannot lose them beside valid calls.
	ResidualProtocolText string `json:"-"`
	// SourceText is the fence-stripped source for an atomic, ordered repair.
	// It is internal bookkeeping, not part of the public parse result.
	SourceText        string `json:"-"`
	RejectedByPolicy  bool
	RejectedToolNames []string
}
