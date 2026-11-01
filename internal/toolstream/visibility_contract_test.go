package toolstream

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/toolcall"
)

func TestSharedVisibilityContractAcrossPartitions(t *testing.T) {
	data, err := os.ReadFile("../../tests/compat/fixtures/visibility/stream.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name, Marker, Raw, Visible string
		Calls                      []toolcall.ParsedToolCall
		Invalid                    bool
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			check := func(chunks []string) {
				t.Helper()
				state := State{DeferMalformed: true, CleanVisible: true, Marker: f.Marker}
				var text strings.Builder
				var deferred strings.Builder
				calls := []toolcall.ParsedToolCall{}
				consume := func(events []Event) {
					for _, e := range events {
						if !e.Deferred {
							text.WriteString(e.Content)
						} else {
							deferred.WriteString(e.Content)
						}
						calls = append(calls, e.ToolCalls...)
					}
				}
				for _, chunk := range chunks {
					consume(ProcessChunk(&state, chunk, []string{"read"}))
				}
				consume(Flush(&state, []string{"read"}))
				if !f.Invalid && len(calls) == 0 {
					text.WriteString(FilterVisible(deferred.String(), f.Marker))
				}
				if text.String() != f.Visible || !reflect.DeepEqual(calls, f.Calls) {
					t.Fatalf("chunks=%q text=%q calls=%#v; want text=%q calls=%#v", chunks, text.String(), calls, f.Visible, f.Calls)
				}
			}
			for split := 0; split <= len(f.Raw); split++ {
				check([]string{f.Raw[:split], f.Raw[split:]})
			}
			for size := 1; size <= len(f.Raw); size++ {
				var chunks []string
				for i := 0; i < len(f.Raw); i += size {
					chunks = append(chunks, f.Raw[i:min(i+size, len(f.Raw))])
				}
				check(chunks)
			}
		})
	}
}
