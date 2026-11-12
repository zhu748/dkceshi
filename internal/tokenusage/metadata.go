package tokenusage

// Metadata accompanies the standard protocol counters. Sources describe each
// field, not merely the response as a whole (input may still be estimated).
type Metadata struct {
	Source          string    `json:"source"`
	InputSource     string    `json:"input_source"`
	OutputSource    string    `json:"output_source"`
	ReasoningSource string    `json:"reasoning_source"`
	TotalSource     string    `json:"total_source"`
	Complete        bool      `json:"complete"`
	Upstream        *Snapshot `json:"upstream,omitempty"`
}
