package promptcompat

import (
	"reflect"

	"ds2api/internal/prompt"
)

// Keep original extension values in the standard model until final rendering.
// This is shared by protocol adapters, history, uploads and internal Chat hops.
func messageContextFields(message map[string]any, consumed ...string) map[string]any {
	fields := prompt.MessageContextFields(message)
	for _, key := range consumed {
		delete(fields, key)
	}
	return fields
}

func copyMessageContextFields(out, source map[string]any, consumed ...string) {
	for key, value := range messageContextFields(source, consumed...) {
		mergeMessageContextField(out, key, value)
	}
}

func mergeMessageContextField(out map[string]any, key string, value any) {
	if fragment, exists := out[key]; exists && !reflect.DeepEqual(fragment, value) {
		out[key] = map[string]any{"source_message": value, "source_fragment": fragment}
	} else {
		out[key] = value
	}
}

// Native tool blocks may carry contextual extensions beside their arguments or
// results. Keep each source container separate instead of flattening collisions.
func PreserveNativeToolContext(out, source map[string]any, container string, consumed ...string) {
	fields := messageContextFields(source, append(consumed, "type")...)
	if len(fields) == 0 {
		return
	}
	context := shallowCopyMap(asContextMap(out["native_tool_context"]))
	context[container] = fields
	out["native_tool_context"] = context
}

func asContextMap(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}
