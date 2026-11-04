package toolcall

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidJSONObjectsAreNeverUnwrapped(t *testing.T) {
	for _, raw := range []string{`{"item":42}`, `{"item":[]}`, `{"options":[1,2]}`, `{"item":{"item":[1]}}`} {
		for _, body := range []string{raw, "<![CDATA[" + raw + "]]>"} {
			calls := ParseToolCalls(`<tool_calls><invoke name="write"><parameter name="options">`+body+`</parameter></invoke></tool_calls>`, nil)
			if len(calls) != 1 {
				t.Fatalf("call lost: %s", body)
			}
			data, _ := json.Marshal(calls[0].Input["options"])
			if string(data) != raw {
				t.Fatalf("JSON object changed: %s -> %s", raw, data)
			}
		}
	}
}

func TestTopLevelSplitCDATAKeepsLiteralTerminators(t *testing.T) {
	for _, name := range []string{"content", "payload"} {
		for _, value := range []string{"a]]>b", "a\x00]]>b", "]]>]]>", "  原文\r\n\t]]> &amp; tail "} {
			encoded := "<![CDATA[" + strings.ReplaceAll(value, "]]>", "]]]]><![CDATA[>") + "]]>"
			calls := ParseToolCalls(`<tool_calls><invoke name="write"><parameter name="`+name+`">`+encoded+`</parameter></invoke></tool_calls>`, nil)
			if len(calls) != 1 || calls[0].Input[name] != value {
				t.Fatalf("CDATA changed: name=%s value=%q calls=%+v", name, value, calls)
			}
		}
	}
}
