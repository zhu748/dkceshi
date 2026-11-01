package toolcall

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Schemas are supplied by clients. Resolve only resources in the schema itself;
// never perform network requests or read local files while compiling a $ref.
type localSchemaLoader struct{}

func (localSchemaLoader) Load(string) (any, error) {
	return nil, fmt.Errorf("external JSON Schema references are not supported")
}

var compiledSchemas = struct {
	sync.Mutex
	entries map[[32]byte]*jsonschema.Schema
	order   [][32]byte
}{entries: map[[32]byte]*jsonschema.Schema{}}

func compileToolSchema(raw any) (*jsonschema.Schema, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256(data)
	compiledSchemas.Lock()
	cached := compiledSchemas.entries[key]
	compiledSchemas.Unlock()
	if cached != nil {
		return cached, nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(localSchemaLoader{})
	const resource = "https://ds2api.invalid/tool-schema.json"
	if err = c.AddResource(resource, doc); err != nil {
		return nil, err
	}
	schema, err := c.Compile(resource)
	if err != nil {
		return nil, err
	}
	// Bound both entry count and cached schema size. Large schemas still validate.
	if len(data) <= 65536 {
		compiledSchemas.Lock()
		defer compiledSchemas.Unlock()
		if compiledSchemas.entries[key] == nil {
			if len(compiledSchemas.order) >= 128 {
				delete(compiledSchemas.entries, compiledSchemas.order[0])
				compiledSchemas.order = compiledSchemas.order[1:]
			}
			compiledSchemas.entries[key] = schema
			compiledSchemas.order = append(compiledSchemas.order, key)
		}
	}
	return schema, nil
}

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
		schema, err := compileToolSchema(meta.schema)
		if err != nil {
			return fmt.Errorf("invalid schema for tool %q: %w", call.Name, err)
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
