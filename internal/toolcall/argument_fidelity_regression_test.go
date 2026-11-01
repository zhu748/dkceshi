package toolcall

import "testing"

func TestParameterEntitiesDecodeOnce(t *testing.T) {
	for _, tc := range []struct{ encoded, want string }{
		{"&amp;amp;", "&amp;"}, {"&amp;lt;", "&lt;"}, {"&#65;&#x1F600;", "A😀"}, {"&apos;&quot;", "'\""},
	} {
		raw := `<tool_calls><invoke name="write"><parameter name="content">` + tc.encoded + `</parameter></invoke></tool_calls>`
		calls := ParseToolCalls(raw, nil)
		if len(calls) != 1 || calls[0].Input["content"] != tc.want {
			t.Fatalf("argument changed: %s => %#v, want %q", tc.encoded, calls, tc.want)
		}
	}
}

func TestValidJSONPathAndLiteralMarkupRemainExact(t *testing.T) {
	raw := `<tool_calls><invoke name="write"><parameter name="options">{"file_path":"first\nsecond","sentinel":"<noop>","literal":"&amp;"}</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(raw, nil)
	if len(calls) != 1 {
		t.Fatalf("parse failed: %#v", calls)
	}
	options, ok := calls[0].Input["options"].(map[string]any)
	if !ok || options["file_path"] != "first\nsecond" || options["sentinel"] != "<noop>" || options["literal"] != "&amp;" {
		t.Fatalf("valid JSON changed: %#v", calls)
	}
}

func TestCDATAArrayRetainsLiteralEntities(t *testing.T) {
	raw := `<tool_calls><invoke name="write"><parameter name="items"><![CDATA[{"text":"&amp;"},{"text":"&lt;"}]]></parameter></invoke></tool_calls>`
	calls := ParseToolCalls(raw, nil)
	if len(calls) != 1 {
		t.Fatalf("parse failed: %#v", calls)
	}
	items, ok := calls[0].Input["items"].([]any)
	if !ok || len(items) != 2 || items[0].(map[string]any)["text"] != "&amp;" {
		t.Fatalf("CDATA changed: %#v", calls)
	}
}

func TestNestedXMLParameterDecodesLeafEntitiesOnce(t *testing.T) {
	raw := `<tool_calls><invoke name="write"><parameter name="options"><label>&amp;amp;</label></parameter></invoke></tool_calls>`
	calls := ParseToolCalls(raw, nil)
	if len(calls) != 1 {
		t.Fatalf("parse failed: %#v", calls)
	}
	options, ok := calls[0].Input["options"].(map[string]any)
	if !ok || options["label"] != "&amp;" {
		t.Fatalf("nested entity changed: %#v", calls)
	}
}
