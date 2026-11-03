package toolcall

import (
	"context"
	"testing"

	"ds2api/internal/tooldiagnostics"
)

func TestRepairDiscardsValidOutputReturnedAfterCancellation(t *testing.T) {
	for _, mode := range []string{"original", "custom"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(tooldiagnostics.WithRepairSettings(context.Background(), tooldiagnostics.RepairSettings{Mode: mode, TimeoutSeconds: 5}))
			defer cancel()
			calls, ok := RepairToolCallsWithLLM(ctx, `<|EPSE|call name="read"></|EPSE|call>`, func(context.Context, string) (string, error) {
				cancel()
				return `<tool_calls><invoke name="read"></invoke></tool_calls>`, nil
			})
			if ok || len(calls) > 0 {
				t.Fatalf("cancelled repair delivered calls: %+v", calls)
			}
		})
	}
}

func TestCDATAParameterPreservesTextAfterClosedSection(t *testing.T) {
	raw := `<tool_calls><invoke name="write"><parameter name="content"><![CDATA[head]]>tail</parameter></invoke></tool_calls>`
	calls := ParseToolCalls(raw, nil)
	if len(calls) != 1 || calls[0].Input["content"] != "headtail" {
		t.Fatalf("CDATA suffix lost: %+v", calls)
	}
}

func TestMixedCDATAStringsKeepLiteralEntitiesAndTypes(t *testing.T) {
	for _, tc := range []struct{ body, want string }{{`<![CDATA[&amp;]]>&amp;`, `&amp;&`}, {`<![CDATA[1]]>23`, `123`}} {
		raw := `<tool_calls><invoke name="write"><parameter name="content">` + tc.body + `</parameter></invoke></tool_calls>`
		calls := ParseToolCalls(raw, nil)
		if len(calls) != 1 || calls[0].Input["content"] != tc.want {
			t.Fatalf("mixed CDATA changed: %+v", calls)
		}
	}
}

func TestUnavailableRepairSettingsNeverCallProvider(t *testing.T) {
	ctx := tooldiagnostics.WithRepairSettings(context.Background(), tooldiagnostics.RepairSettings{Mode: "custom", UnavailableReason: "invalid saved parameters"})
	calls, ok := RepairToolCallsWithLLM(ctx, `<|EPSE|call name="read"></|EPSE|call>`, func(context.Context, string) (string, error) {
		t.Fatal("suspended repair reached provider")
		return "", nil
	})
	if ok || len(calls) > 0 {
		t.Fatal("suspended repair succeeded")
	}
}

func TestUnavailableRepairStillAllowsLocalDSMLRecovery(t *testing.T) {
	ctx := tooldiagnostics.WithRepairSettings(context.Background(), tooldiagnostics.RepairSettings{Mode: "custom", UnavailableReason: "invalid saved parameters"})
	raw := nativeDSMLCall(`<｜｜DSML｜｜ parameter name="path" string="true">safe.txt</｜｜DSML｜｜ parameter>`)
	calls, ok := RepairToolCallsWithLLM(ctx, raw, func(context.Context, string) (string, error) {
		t.Fatal("local recovery reached provider")
		return "", nil
	})
	if !ok || len(calls) != 1 || calls[0].Input["path"] != "safe.txt" {
		t.Fatalf("local recovery incorrectly suspended: %+v", calls)
	}
}
