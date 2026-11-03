package promptcompat

import "testing"

func TestReferenceFileTokensAreDeduplicatedScopedAndTrusted(t *testing.T) {
	req := map[string]any{"file_ids": []any{"file-a", "file-a", "file-zero"}, "_inline_file_bytes": 90000}
	SetReferenceFileTokens(req, map[string]int{"file-a": 954, "file-zero": 0, "unreferenced": 500})
	if got := EstimateInlineFileTokens(req); got != 954 {
		t.Fatalf("expected exact deduplicated file usage, got %d", got)
	}
	req["file_ids"] = []any{"file-zero"}
	if got := EstimateInlineFileTokens(req); got != 0 {
		t.Fatalf("zero count fell back to bytes: %d", got)
	}
	req[referenceFileUsageKey] = map[string]any{"file-zero": 900}
	delete(req, "_inline_file_bytes")
	if got := EstimateInlineFileTokens(req); got != 0 {
		t.Fatalf("JSON metadata spoofed trusted counts: %d", got)
	}
}
