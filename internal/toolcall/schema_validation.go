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
		if schema == nil || tool["type"] == "custom" {
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
	_, err := validateToolArguments(calls, toolsRaw, false)
	return err
}

// ArgumentValidation holds the actual per-call result for reuse by diagnostics.
type ArgumentValidation struct {
	HasSchema bool
	Checked   bool
	Error     error
}

func ValidateToolArgumentsDetailed(calls []ParsedToolCall, toolsRaw any) ([]ArgumentValidation, error) {
	return validateToolArguments(calls, toolsRaw, true)
}

type argumentSchema struct {
	schema any
	custom bool
}
type argumentValidator struct {
	catalog  map[string]argumentSchema
	compiled map[string]*jsonschema.Schema
}

func validateToolArguments(calls []ParsedToolCall, toolsRaw any, detailed bool) ([]ArgumentValidation, error) {
	if len(calls) > streambudget.Value("max_tool_calls") {
		return nil, fmt.Errorf("tool call batch exceeds configured limit")
	}
	if err := validateToolCatalog(toolsRaw); err != nil {
		return nil, err
	}
	tools, _ := toolsRaw.([]any)
	v := argumentValidator{catalog: make(map[string]argumentSchema, len(tools)), compiled: make(map[string]*jsonschema.Schema)}
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		name, _, schema := ExtractToolMeta(tool)
		v.catalog[name] = argumentSchema{schema: schema, custom: tool["type"] == "custom"}
	}
	var results []ArgumentValidation
	if detailed {
		results = make([]ArgumentValidation, 0, len(calls))
	}
	var firstError error
	for _, call := range calls {
		result := v.validate(call)
		if detailed {
			results = append(results, result)
		}
		if result.Error != nil && !detailed {
			return nil, result.Error
		}
		if firstError == nil {
			firstError = result.Error
		}
	}
	return results, firstError
}
func (v *argumentValidator) validate(call ParsedToolCall) (result ArgumentValidation) {
	meta, declared := v.catalog[call.Name]
	result.Checked, result.HasSchema = true, meta.schema != nil && !meta.custom
	if len(v.catalog) > 0 && !declared {
		result.Error = fmt.Errorf("undeclared tool %q", call.Name)
		return result
	}
	if meta.custom {
		if _, ok := call.Input["input"].(string); !ok || len(call.Input) != 1 {
			result.Error = fmt.Errorf("custom tool %q requires one string input", call.Name)
		}
		return result
	}
	if meta.schema == nil {
		return result
	}
	schema := v.compiled[call.Name]
	if schema == nil {
		var err error
		schema, err = compileToolSchema(meta.schema)
		if err != nil {
			result.Error = fmt.Errorf("invalid schema for tool %q: %w", call.Name, err)
			return result
		}
		v.compiled[call.Name] = schema
	}
	data, err := json.Marshal(call.Input)
	if err != nil {
		result.Error = err
		return result
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		result.Error = err
		return result
	}
	if err = schema.Validate(value); err != nil {
		result.Error = fmt.Errorf("invalid arguments for tool %q: %w", call.Name, err)
	}
	return result
}
