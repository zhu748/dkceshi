package promptcompat

import (
	"reflect"
	"strings"
	"testing"
)

func TestConflictingUploadedReferenceAliasesRejected(t *testing.T) {
	for _, block := range []map[string]any{
		{"type": "input_file", "file_id": "file-A", "id": "file-B"},
		{"type": "input_file", "file_id": "file-A", "file": map[string]any{"id": "file-B"}},
		{"type": "input_file", "file": map[string]any{"file_id": "file-A", "id": "file-B"}},
		{"type": "input_image", "file_id": "file-A", "file": map[string]any{"file_id": "file-B"}},
		{"type": "input_file", "file_id": false},
	} {
		request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
		if _, err := NormalizeOpenAIChatRequest(nil, request, ""); err == nil {
			t.Fatalf("ambiguous reference accepted: %#v", block)
		}
	}
}

func TestUploadedReferenceAliasesUseOneIdentityWithoutLosingLabels(t *testing.T) {
	request := map[string]any{"model": "deepseek-v4.1-flash", "attachments": []any{map[string]any{"id": "file-bag", "filename": "bag.txt"}}, "messages": []any{map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "input_file", "file_id": " file-A ", "file": map[string]any{"filename": "source.txt"}, "description": "REFERENCE_DESCRIPTION"},
	}}}}
	standard, err := NormalizeOpenAIChatRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(standard.RefFileIDs, []string{"file-bag", "file-A"}) || strings.Count(standard.FinalPrompt, "[Attachment reference]") != 2 {
		t.Fatalf("inconsistent reference identity: %v %s", standard.RefFileIDs, standard.FinalPrompt)
	}
	for _, text := range []string{standard.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages)} {
		for _, want := range []string{"file-bag", "file-A", "source.txt", "bag.txt", "REFERENCE_DESCRIPTION"} {
			if !strings.Contains(text, want) {
				t.Fatalf("reference rendering lost %s", want)
			}
		}
	}
}

func TestResponsesCompatibilityArrayPreservesBlankTextPieces(t *testing.T) {
	request := map[string]any{"model": "deepseek-v4.1-flash", "instructions": " \r\n\t", "input": []any{"first", " \r\n\t", "", "last"}}
	standard, err := NormalizeOpenAIResponsesRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(standard.FinalPrompt, "[System]: \r\n\t\n") || !strings.Contains(standard.FinalPrompt, "[User]:first\n \r\n\t\n\nlast") {
		t.Fatalf("blank source fragments disappeared: %q", standard.FinalPrompt)
	}
}

func TestResponsesExplicitEmptyAndWhitespaceInputsKeepTheirMessage(t *testing.T) {
	for _, input := range []any{"", " \r\n\t", []any{""}, []any{" \r\n\t"}} {
		standard, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": input}, "")
		if err != nil || len(standard.Messages) != 1 {
			t.Fatalf("explicit input lost its message boundary: %#v, %v", input, err)
		}
	}
}
