package files

import (
	"net/http"
	"reflect"

	"ds2api/internal/promptcompat"
)

func (s *inlineUploadState) walkRequestInputs(req map[string]any) error {
	camel, hasCamel := req["systemInstruction"]
	snake, hasSnake := req["system_instruction"]
	aliased := hasCamel && hasSnake
	if aliased && !reflect.DeepEqual(camel, snake) {
		if _, _, err := promptcompat.ResolvePayloadAliases(req, "systemInstruction", "system_instruction"); err != nil {
			return &inlineFileUploadError{status: http.StatusBadRequest, message: err.Error(), err: err}
		}
	}
	for _, key := range promptcompat.InlineFileInputKeys() {
		if key == "system_instruction" && aliased {
			req[key] = req["systemInstruction"]
			continue
		}
		if raw, ok := req[key]; ok {
			updated, err := s.walk(raw, key == "attachments" || key == "files")
			if err != nil {
				return err
			}
			req[key] = updated
		}
	}
	return nil
}
