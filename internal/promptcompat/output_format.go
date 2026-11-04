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
	if nested, ok := object["json_schema"].(map[string]any); ok {
		format.Schema = nested["schema"]
	}
	if err := format.Check(); err != nil {
		return req, fmt.Errorf("invalid output format: %w", err)
	}
	req.OutputFormat = format
	req.FinalPrompt, req.ToolNames = req.BuildPromptFromMessages(req.Messages)
	req.PromptTokenText = req.FinalPrompt
	return req, nil
}

func applyOpenAIOutputFormat(req StandardRequest, raw map[string]any) (StandardRequest, error) {
	format := raw["response_format"]
	if text, ok := raw["text"].(map[string]any); ok && text["format"] != nil {
		if format != nil && !reflect.DeepEqual(format, text["format"]) {
			return req, fmt.Errorf("response_format conflicts with text.format")
		}
		format = text["format"]
	}
	return ApplyOutputFormat(req, format)
}

func (r StandardRequest) withOutputInstruction(text string) string {
	return prompt.PrependSystemInstruction(text, r.OutputFormat.Instruction())
}
