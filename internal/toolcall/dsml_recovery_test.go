package toolcall

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func nativeDSMLCall(body string) string {
	return `<｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="lookup">` + body + `</｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls>`
}

func TestDSMLRecoveryPreservesTypesAndLiteralBytesWithoutLLM(t *testing.T) {
	literal := "  00123 &amp; <b>text</b>\n"
	params := `<｜｜DSML｜｜ parameter name="literal" string="true">` + literal + `</｜｜DSML｜｜ parameter>` +
		`<｜｜DSML｜｜ parameter name="id" string="false">9007199254740993</｜｜DSML｜｜ parameter>` +
		`<｜｜DSML｜｜ parameter name="options" string="false">{"items":[true,null,[]]}</｜｜DSML｜｜ parameter>`
	invoked := false
	calls, ok := RepairToolCallsWithLLM(context.Background(), nativeDSMLCall(params), func(context.Context, string) (string, error) {
		invoked = true
		return "", nil
	})
	if !ok || invoked || len(calls) != 1 || calls[0].Input["literal"] != literal || calls[0].Input["id"] != json.Number("9007199254740993") {
		t.Fatalf("ok=%v invoked=%v calls=%#v", ok, invoked, calls)
	}
	if !reflect.DeepEqual(calls[0].Input["options"], map[string]any{"items": []any{true, nil, []any{}}}) {
		t.Fatalf("nested values changed: %#v", calls[0].Input)
	}
	withProse := "before " + nativeDSMLCall(params) + " after"
	if got, recovered := RecoverDSMLToolCalls(withProse); !recovered || !reflect.DeepEqual(got, calls) {
		t.Fatalf("prose changed recovery: %#v", got)
	}
	singlePipes := strings.ReplaceAll(nativeDSMLCall(params), "｜｜DSML｜｜", "｜DSML｜")
	if got, recovered := RecoverDSMLToolCalls(singlePipes); !recovered || !reflect.DeepEqual(got, calls) {
		t.Fatalf("native single-pipe spelling changed recovery: %#v", got)
	}
	cdataLiteral := `<![CDATA[<b>literal &amp; text</b>]]>`
	input := nativeDSMLCall(`<｜｜DSML｜｜ parameter name="literal" string="true">` + cdataLiteral + `</｜｜DSML｜｜ parameter>`)
	if got, recovered := RecoverDSMLToolCalls(input); !recovered || got[0].Input["literal"] != cdataLiteral {
		t.Fatalf("native raw CDATA literal was rewritten: %#v", got)
	}
}

func TestDSMLRecoveryRejectsIncompleteOrAmbiguousInput(t *testing.T) {
	param := `<｜｜DSML｜｜ parameter name="value" string="true">x</｜｜DSML｜｜ parameter>`
	valid := nativeDSMLCall(param)
	for _, input := range []string{
		strings.TrimSuffix(valid, `</｜｜DSML｜｜ calls>`),
		nativeDSMLCall(strings.Replace(param, ` string="true"`, "", 1)),
		nativeDSMLCall(strings.Replace(param, `string="true"`, `string="maybe"`, 1)),
		nativeDSMLCall(strings.Replace(param, `string="true"`, `string="false"`, 1)),
		nativeDSMLCall(param + param),
		nativeDSMLCall(strings.Replace(param, ">x<", "><![CDATA[unterminated<", 1)),
		strings.Replace(valid, `name="lookup"`, `name="lookup" name="other"`, 1),
		strings.Replace(valid, `</｜｜DSML｜｜ calls>`, `</｜｜DSML｜｜ calls extra="x">`, 1),
		valid + `<|EPSE|call name="other"></|EPSE|call>`,
		"```xml\n" + valid + "\n```",
		"`" + valid + "`",
	} {
		if calls, ok := RecoverDSMLToolCalls(input); ok || len(calls) > 0 {
			t.Fatalf("ambiguous input recovered: %q calls=%#v", input, calls)
		}
	}
}
