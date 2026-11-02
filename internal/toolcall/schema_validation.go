package toolcall

import (
	"bytes"
	"ds2api/internal/streambudget"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func ValidateToolSchemas(toolsRaw any) error {
	if err := validateToolCatalog(toolsRaw); err != nil {
		return err
	}
	tools, _ := toolsRaw.([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		name, _, schema := ExtractToolMeta(tool)
		if schema == nil || IsCustomTool(toolsRaw, name) {
			continue
		}
		if _, err := compileToolSchema(schema); err != nil {
			return fmt.Errorf("invalid schema for tool %q: %w", name, err)
		}
	}
	return nil
}

// ValidateToolArguments runs after deterministic normalization and before any
// executable call is delivered, for both ordinary calls and LLM-repaired ones.
func ValidateToolArguments(calls []ParsedToolCall, toolsRaw any) error {
	if len(calls) > streambudget.Value("max_tool_calls") {
		return fmt.Errorf("tool call batch exceeds configured limit")
	}
	if err := validateToolCatalog(toolsRaw); err != nil {
		return err
	}
	tools, _ := toolsRaw.([]any)
	type entry struct {
		schema any
		custom bool
	}
	catalog := make(map[string]entry, len(tools))
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		name, _, schema := ExtractToolMeta(tool)
		catalog[name] = entry{schema: schema, custom: tool["type"] == "custom"}
	}
	compiled := make(map[string]*jsonschema.Schema, len(catalog))
	for _, call := range calls {
		meta, declared := catalog[call.Name]
		if len(catalog) > 0 && !declared {
			return fmt.Errorf("undeclared tool %q", call.Name)
		}
		if meta.custom {
			if _, ok := call.Input["input"].(string); !ok || len(call.Input) != 1 {
				return fmt.Errorf("custom tool %q requires one string input", call.Name)
			}
			continue
		}
		if meta.schema == nil {
			continue
		}
		schema := compiled[call.Name]
		if schema == nil {
			var err error
			schema, err = compileToolSchema(meta.schema)
			if err != nil {
				return fmt.Errorf("invalid schema for tool %q: %w", call.Name, err)
			}
			compiled[call.Name] = schema
		}
		data, err := json.Marshal(call.Input)
		if err != nil {
			return err
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if err = schema.Validate(value); err != nil {
			return fmt.Errorf("invalid arguments for tool %q: %w", call.Name, err)
		}
	}
	return nil
}
