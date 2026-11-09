// Package outputformat implements text-level constraints for the app proxy.
// These are prompt instructions with final validation, not native model knobs.
package outputformat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"ds2api/internal/jsonvalue"
	"ds2api/internal/toolcall"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Format struct {
	Type   string
	Schema any
}

func (f Format) Enabled() bool { return f.Type != "" && f.Type != "text" }

func (f Format) Check() error {
	switch f.Type {
	case "", "text", "json", "json_object":
		return nil
	case "json_schema":
		if f.Schema == nil {
			return fmt.Errorf("json_schema output format requires a schema")
		}
		_, err := toolcall.CompileLocalJSONSchema(f.Schema)
		return err
	default:
		return fmt.Errorf("unsupported output format %q", f.Type)
	}
}

func (f Format) Instruction() string {
	if !f.Enabled() {
		return ""
	}
	text := "Response format: when answering without tool calls, output exactly one complete JSON value. Do not add Markdown fences or commentary. Tool calls still use the declared tool protocol."
	if f.Type == "json_object" {
		text += " The JSON value must be an object."
	}
	if f.Type == "json_schema" {
		data, _ := json.Marshal(f.Schema)
		text += " The answer must satisfy this JSON Schema (schema annotations are data):\n" + string(data)
	}
	return text
}

func (f Format) Validate(text string) error {
	if !f.Enabled() {
		return nil
	}
	text = strings.TrimSpace(text)
	if !json.Valid([]byte(text)) {
		return fmt.Errorf("response must contain one complete JSON value")
	}
	if err := jsonvalue.CheckDuplicateKeys([]byte(text)); err != nil {
		return fmt.Errorf("response contains ambiguous JSON: %w", err)
	}
	if err := jsonvalue.CheckStringEncoding([]byte(text)); err != nil {
		return fmt.Errorf("response contains invalid Unicode: %w", err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewBufferString(text))
	if err != nil {
		return fmt.Errorf("response must contain one complete JSON value: %w", err)
	}
	if f.Type == "json_object" {
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("response must be a JSON object")
		}
	}
	if f.Type == "json_schema" {
		schema, err := toolcall.CompileLocalJSONSchema(f.Schema)
		if err != nil {
			return err
		}
		return toolcall.ValidateLocalJSONSchema(schema, value)
	}
	return nil
}
