package promptcompat

import "math"

// The private Go type keeps JSON request fields from impersonating resolved
// server metadata. Counts are scoped to this request and its selected account.
type referenceFileUsage map[string]int

const referenceFileUsageKey = "_reference_file_usage"

func SetReferenceFileTokens(req map[string]any, counts map[string]int) {
	usage := make(referenceFileUsage, len(counts))
	for id, tokens := range counts {
		if tokens >= 0 {
			usage[id] = tokens
		}
	}
	req[referenceFileUsageKey] = usage
}

func referenceFileTokens(req map[string]any) (int, bool) {
	usage, ok := req[referenceFileUsageKey].(referenceFileUsage)
	if !ok {
		return 0, false
	}
	total := 0
	for _, id := range CollectOpenAIRefFileIDs(req) {
		tokens := usage[id]
		if tokens > math.MaxInt-total {
			return math.MaxInt, true
		}
		total += tokens
	}
	return total, true
}
