package files

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestReplayKeepsRequestPolicyAndDoesNotSerializeSource(t *testing.T) {
	u := &nativeFileUploader{}
	snapshot := &inlineReplaySnapshot{
		source:   map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_file", "filename": "secret.txt", "file_data": "QUJDRA=="}}}}},
		uploader: u, model: "deepseek-v4.1-flash", owner: "old",
	}
	standard := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", ToolChoice: promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceRequired}, Thinking: true, ToolMarker: "original"}
	standard.InputContext = promptcompat.InputFieldDigests(map[string]any{"input": "original context"})
	standard = WithReplay(standard, &auth.RequestAuth{}, map[string]any{replaySnapshotKey: snapshot}, func(_ context.Context, req map[string]any) (promptcompat.StandardRequest, error) {
		return promptcompat.StandardRequest{ResolvedModel: "changed", ToolChoice: promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceNone}, Messages: req["messages"].([]any), RefFileIDs: promptcompat.CollectOpenAIRefFileIDs(req)}, nil
	})
	encoded, err := json.Marshal(standard)
	if err != nil || strings.Contains(string(encoded), "QUJDRA==") || strings.Contains(string(encoded), replaySnapshotKey) {
		t.Fatalf("serialized replay state: %s error=%v", encoded, err)
	}
	out, err := standard.ReplayInput(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.ResolvedModel != standard.ResolvedModel || out.ToolChoice.Mode != promptcompat.ToolChoiceRequired || !out.Thinking || out.ToolMarker != "original" {
		t.Fatal("changed request policy during replay")
	}
	if len(out.InputContext) != 1 || out.InputContext[0] != standard.InputContext[0] {
		t.Fatal("attachment replay replaced source context evidence")
	}
	if len(u.uploads) != 1 || string(u.uploads[0].Data) != "ABCD" || len(out.RefFileIDs) != 1 {
		t.Fatal("source attachment not rebuilt")
	}
}
