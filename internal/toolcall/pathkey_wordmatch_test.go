package toolcall

import "testing"

func TestParseToolCallInputKeepsProfileControlChars(t *testing.T) {
	// "profile" must not be treated as a file path: its real newline survives.
	in := `{"profile": "line1\nline2", "path": "D:\\tmp\\new\\readme.txt"}`
	parsed := parseToolCallInput(in)
	profile, ok := parsed["profile"].(string)
	if !ok {
		t.Fatalf("expected profile string, got %#v", parsed["profile"])
	}
	if profile != "line1\nline2" {
		t.Fatalf("profile newline must survive, got %q", profile)
	}
	path, ok := parsed["path"].(string)
	if !ok {
		t.Fatalf("expected path string, got %#v", parsed["path"])
	}
	if path != `D:\tmp\new\readme.txt` {
		t.Fatalf("expected repaired windows path, got %q", path)
	}
}
