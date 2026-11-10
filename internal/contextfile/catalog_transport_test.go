package contextfile

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestExternalizedCatalogRetainsExtensionsAndMessageRoutesOnReplay(t *testing.T) {
	request := map[string]any{"model": "deepseek-v4.1-flash", "tools": []any{map[string]any{"name": "write", "parameters": map[string]any{"type": "object"}, "usage_policy": "CATALOG_SOURCE_RULE"}}, "messages": []any{
		map[string]any{"role": "system", "content": "SYSTEM_RULE"},
		map[string]any{"role": "user", "content": "OLD_TASK", "metadata": "OLD_METADATA"},
		map[string]any{"role": "assistant", "content": "DONE"},
		map[string]any{"role": "user", "content": "NEW_TASK"},
	}}
	std, err := promptcompat.NormalizeOpenAIChatRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	uploader := &recordingUploader{}
	service := Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}
	split, err := service.ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, std)
	if err != nil || !split.CurrentInputFileApplied {
		t.Fatal("history split failed", err)
	}
	tools, ok := uploader.uploadFor(currentToolsFilename)
	if !ok || !strings.Contains(string(tools.Data), "CATALOG_SOURCE_RULE") || strings.Contains(split.FinalPrompt, "CATALOG_SOURCE_RULE") {
		t.Fatal("tool context was lost or duplicated across transports")
	}
	manifest := split.ContextManifest()
	for _, route := range manifest.MessageTransports {
		if route.OmittedReason != "" || route.LiveIndex == nil && route.HistoryIndex == nil {
			t.Fatalf("source message lost: %+v", route)
		}
	}
	for _, field := range manifest.ToolFields {
		if field.Path == "/tools/0/usage_policy" && !reflect.DeepEqual(field.Destinations, []string{"tools_file"}) {
			t.Fatalf("tool context has stale destination: %+v", field)
		}
	}
	second := &recordingUploader{}
	replayed, err := (Service{DS: second}).ReuploadAppliedCurrentInputFile(context.Background(), &auth.RequestAuth{}, split)
	if err != nil {
		t.Fatal(err)
	}
	newTools, _ := second.uploadFor(currentToolsFilename)
	if string(newTools.Data) != string(tools.Data) || !reflect.DeepEqual(replayed.ContextManifest().MessageTransports, manifest.MessageTransports) {
		t.Fatal("account replay changed text or source mapping")
	}
}
