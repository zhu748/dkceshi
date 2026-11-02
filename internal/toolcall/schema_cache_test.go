package toolcall

import (
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestConcurrentSchemaCompilesShareTheResult(t *testing.T) {
	raw := map[string]any{"type": "object", "description": t.Name(), "properties": map[string]any{"id": map[string]any{"type": "string"}}}
	const n = 64
	results := make([]*jsonschema.Schema, n)
	errors := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; results[i], errors[i] = compileToolSchema(raw) }(i)
	}
	close(start)
	wg.Wait()
	for i := range n {
		if errors[i] != nil || results[i] == nil || results[i] != results[0] {
			t.Fatalf("duplicate/failed compile %d: %p %v", i, results[i], errors[i])
		}
	}
}

func TestBatchSchemaReuseStillValidatesEveryInvocation(t *testing.T) {
	tools := []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "required": []any{"path"}, "properties": map[string]any{"path": map[string]any{"type": "string"}}}}}
	calls := make([]ParsedToolCall, 64)
	for i := range calls {
		calls[i] = ParsedToolCall{Name: "read", Input: map[string]any{"path": "a.txt"}}
	}
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal(err)
	}
	calls[len(calls)-1].Input = map[string]any{}
	if err := ValidateToolArguments(calls, tools); err == nil {
		t.Fatal("last invocation escaped validation")
	}
}
