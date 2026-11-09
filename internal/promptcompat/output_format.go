package promptcompat

import (
	"fmt"
	"reflect"

	"ds2api/internal/outputformat"
	"ds2api/internal/prompt"
)

func ApplyOutputFormat(req StandardRequest, raw any) (StandardRequest, error) {
	if raw == nil {
		return req, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return req, fmt.Errorf("output format must be an object")
	}
	typ, ok := object["type"].(string)
	if !ok || typ == "" {
		return req, fmt.Errorf("output format requires a type")
	}
	format := outputformat.Format{Type: typ, Schema: object["schema"]}
	if rawNested, present := object["json_schema"]; present {
		nested, ok := rawNested.(map[string]any)
		if !ok {
			return req, fmt.Errorf("json_schema must be an object")
		}
		if _, present := object["schema"]; present {
			if err := validatePayloadAliases(map[string]any{"schema": object["schema"], "nested_schema": nested["schema"]}, "schema", "nested_schema"); err != nil {
				return req, fmt.Errorf("conflicting output schema aliases: %w", err)
			}
		}
		format.Schema = nested["schema"]
	}
	if err := format.Check(); err != nil {
		return req, fmt.Errorf("invalid output format: %w", err)
	}
	// First-time format application can reuse the serialized conversation.
	// Replacing an existing constraint or a thinking prelude needs a rebuild.
	rebuild := req.OutputFormat.Enabled() || req.ThinkingInjectionPrompt != "" || req.FinalPrompt == ""
	req.OutputFormat = format
	if rebuild {
		req.FinalPrompt, req.ToolNames = req.BuildPromptFromMessages(req.Messages)
	} else {
		req.FinalPrompt = req.withOutputInstruction(req.FinalPrompt)
	}
	req.PromptTokenText = req.FinalPrompt
	return req, nil
}

func applyOpenAIOutputFormat(req StandardRequest, raw map[string]any) (StandardRequest, error) {
	format := raw["response_format"]
	if value := raw["text"]; value != nil {
		text, ok := value.(map[string]any)
		if !ok {
			return req, fmt.Errorf("text must be an output configuration object")
		}
		if value, present := text["format"]; present {
			if _, provided := raw["response_format"]; provided && !reflect.DeepEqual(format, value) {
				return req, fmt.Errorf("response_format conflicts with text.format")
			}
			format = value
		}
	}
	return ApplyOutputFormat(req, format)
}

func (r StandardRequest) withOutputInstruction(text string) string {
	return prompt.PrependSystemInstruction(text, r.OutputFormat.Instruction())
}
