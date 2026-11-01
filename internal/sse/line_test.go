package sse

import (
	"encoding/json"
	"testing"
)

func TestParseDeepSeekContentLineDone(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte("data: [DONE]"), false, "text")
	if !res.Parsed || !res.Stop {
		t.Fatalf("expected parsed stop result: %#v", res)
	}
}

func TestParseDeepSeekContentLineError(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"error":"boom"}`), false, "text")
	if !res.Parsed || !res.Stop {
		t.Fatalf("expected stop on error: %#v", res)
	}
	if res.ErrorMessage == "" {
		t.Fatalf("expected non-empty error message")
	}
}

func TestParseDeepSeekContentLineContentFilter(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"code":"content_filter"}`), false, "text")
	if !res.Parsed || !res.Stop || !res.ContentFilter {
		t.Fatalf("expected content-filter stop result: %#v", res)
	}
}

func TestParseDeepSeekContentLineContentFilterCodeStops(t *testing.T) {
	res := ParseDeepSeekContentLine(
		[]byte(`data: {"code":"content_filter","accumulated_token_usage":99}`),
		false, "text",
	)
	if !res.Parsed || !res.Stop || !res.ContentFilter {
		t.Fatalf("expected content-filter stop result: %#v", res)
	}
}

func TestParseDeepSeekContentLineContentFilterStatus(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/status","v":"CONTENT_FILTER"}`), false, "text")
	if !res.Parsed || !res.Stop || !res.ContentFilter {
		t.Fatalf("expected status-based content-filter stop result: %#v", res)
	}
}

func TestParseDeepSeekContentLineIgnoresAccumulatedTokenUsage(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response","o":"BATCH","v":[{"p":"accumulated_token_usage","v":1383},{"p":"quasi_status","v":"FINISHED"}]}`), false, "text")
	if !res.Parsed {
		t.Fatalf("expected parsed result")
	}
}

func TestParseDeepSeekContentLineIgnoresAccumulatedTokenUsageString(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response","o":"BATCH","v":[{"p":"accumulated_token_usage","v":"190"},{"p":"quasi_status","v":"FINISHED"}]}`), false, "text")
	if !res.Parsed {
		t.Fatalf("expected parsed result")
	}
}

func TestParseDeepSeekContentLineErrorStops(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"error":"boom","accumulated_token_usage":123}`), false, "text")
	if !res.Parsed || !res.Stop {
		t.Fatalf("expected stop on error: %#v", res)
	}
}

func TestParseDeepSeekContentLineContent(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/content","v":"hi"}`), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 1 || res.Parts[0].Text != "hi" || res.Parts[0].Type != "text" {
		t.Fatalf("unexpected parts: %#v", res.Parts)
	}
}

func TestParseDeepSeekContentLineFiltersIncompleteStatusText(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"p":"response/status","v":"INCOMPLETE"}`), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 0 {
		t.Fatalf("expected INCOMPLETE status to be filtered, got %#v", res.Parts)
	}
}

func TestParseDeepSeekContentLinePreservesSpaceOnlyChunk(t *testing.T) {
	res := ParseDeepSeekContentLine([]byte(`data: {"v":" "}`), false, "text")
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result: %#v", res)
	}
	if len(res.Parts) != 1 || res.Parts[0].Text != " " || res.Parts[0].Type != "text" {
		t.Fatalf("unexpected parts for space-only chunk: %#v", res.Parts)
	}
}

func TestParseDeepSeekContentLinePreservesContentFilterText(t *testing.T) {
	for _, text := range []string{"正常输出CONTENT_FILTER你好，这个问题我暂时无法回答", "CONTENT_FILTER你好，这个问题我暂时无法回答", "模型会在命中 CONTENT_FILTER 时返回拒绝原因。", "content_filter", "line1\nCONTENT_FILTERblocked", "\nCONTENT_FILTERblocked"} {
		data, err := json.Marshal(map[string]any{"p": "response/content", "v": text})
		if err != nil {
			t.Fatal(err)
		}
		res := ParseDeepSeekContentLine(append([]byte("data: "), data...), false, "text")
		if !res.Parsed || res.Stop || res.ContentFilter || len(res.Parts) != 1 || res.Parts[0].Text != text {
			t.Fatalf("raw content was changed: %+v", res)
		}
	}
}

func TestParseDeepSeekContentLineHintError(t *testing.T) {
	res := ParseDeepSeekContentLine(
		[]byte(`data: {"type":"error","content":"内容超长，请删减后再试","clear_response":true,"finish_reason":"input_exceeds_limit"}`),
		false, "text",
	)
	if !res.Parsed || !res.Stop {
		t.Fatalf("expected parsed stop result: %#v", res)
	}
	if res.ErrorMessage != "内容超长，请删减后再试" {
		t.Fatalf("expected upstream error message, got %q", res.ErrorMessage)
	}
	if res.ContentFilter {
		t.Fatalf("did not expect content filter for hint error")
	}
}

func TestParseDeepSeekContentLineHintErrorNoContentIgnored(t *testing.T) {
	res := ParseDeepSeekContentLine(
		[]byte(`data: {"type":"error","content":""}`),
		false, "text",
	)
	if !res.Parsed || res.Stop {
		t.Fatalf("expected parsed non-stop result for empty-content hint: %#v", res)
	}
}
