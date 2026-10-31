package toolcall

import "testing"

func TestIsPathLikeKeyWordMatching(t *testing.T) {
	yes := []string{"path", "Path", "file", "file_path", "filePath", "filename", "FileName", "paths", "dir_path", "abs_path", "file-name", "input files", "outputFile", "logFile"}
	no := []string{"profile", "Profile", "xpath", "critical_path_analysis", "profiling", "filet", "pathe", "myfile", "content", "text", "file_hash", "path_analysis", "filesCount"}
	for _, k := range yes {
		if !isPathLikeKey(k) {
			t.Errorf("expected %q to be path-like", k)
		}
	}
	for _, k := range no {
		if isPathLikeKey(k) {
			t.Errorf("expected %q NOT to be path-like", k)
		}
	}
}

func TestParseToolCallInputKeepsProfileControlChars(t *testing.T) {
	// "profile" must not be treated as a file path: its real newline survives.
	in := `{"profile": "line1\nline2", "path": "D:\tmp\new\readme.txt"}`
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
