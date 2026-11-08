package responses

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestContinuationKeepsAttachmentsOnTheirOriginalUserTurns(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	h := &Handler{Store: store, Auth: resolver, DS: &continuationFilesDS{}}
	previous := ""
	for i, task := range []string{"TASK_ALPHA", "TASK_BETA", "TASK_GAMMA"} {
		request := map[string]any{"model": "deepseek-v4.1-flash", "input": task}
		if previous != "" {
			request["previous_response_id"] = previous
		}
		if i < 2 {
			request["attachments"] = []any{map[string]any{"type": "input_file", "file_id": "file-" + task, "description": "ATTACHMENT_" + task}}
		}
		data, _ := json.Marshal(request)
		r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(data)))
		r.Header.Set("Authorization", "Bearer direct-token")
		rec := httptest.NewRecorder()
		h.Responses(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatal(rec.Body.String())
		}
		previous = decodeJSONBody(t, rec.Body.String())["id"].(string)
		owner := responseStoreOwner(authForToken(t, resolver, "direct-token"))
		messages, files, ok := h.getResponseStore().continuationContext(owner, previous)
		if !ok {
			t.Fatal("response context was not stored")
		}
		for _, wantTask := range []string{"TASK_ALPHA", "TASK_BETA"}[:min(i+1, 2)] {
			found := false
			for _, raw := range messages {
				message, _ := raw.(map[string]any)
				text := promptcompat.RenderPromptText([]any{message}, false)
				if strings.Contains(text, "[User]:"+wantTask) {
					found = strings.Count(text, "ATTACHMENT_"+wantTask) == 1
				}
			}
			if !found {
				t.Fatalf("round %d detached attachment from %s", i, wantTask)
			}
		}
		for _, key := range promptcompat.AttachmentInputKeys() {
			if files[key] != nil {
				t.Fatal("message-owned attachment remained in the request-wide replay bag")
			}
		}
	}
}

func TestLegacyContinuationBagsMigrateWithoutLosingReplayBytes(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	h := &Handler{Store: store, Auth: resolver}
	owner := responseStoreOwner(authForToken(t, resolver, "direct-token"))
	legacy := map[string]any{"files": []any{map[string]any{"type": "input_file", "filename": "legacy.txt", "description": "LEGACY_DESCRIPTION", "file_data": "data:text/plain;base64,TEVHQUNZ"}}}
	if !h.getResponseStore().putContext(owner, "legacy-response", map[string]any{}, []any{
		map[string]any{"role": "user", "content": "LEGACY_TASK"}, map[string]any{"role": "assistant", "content": "LEGACY_ANSWER"},
	}, legacy) {
		t.Fatal("could not create a legacy replay record")
	}
	for _, persist := range []bool{false, true} {
		request := map[string]any{"previous_response_id": "legacy-response", "input": "CURRENT_TASK", "store": persist, "attachments": []any{map[string]any{"type": "input_file", "filename": "current.txt", "description": "CURRENT_DESCRIPTION", "file_data": "data:text/plain;base64,Q1VSUkVOVA=="}}}
		r := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		r.Header.Set("Authorization", "Bearer direct-token")
		replay, err := h.restoreContinuation(r, request)
		if err != nil {
			t.Fatal(err)
		}
		messages := request["messages"].([]any)
		oldText := promptcompat.RenderPromptText(messages[:1], false)
		newText := promptcompat.RenderPromptText(messages[len(messages)-1:], false)
		if !strings.Contains(oldText, "LEGACY_DESCRIPTION") || strings.Contains(newText, "LEGACY_DESCRIPTION") || !strings.Contains(newText, "CURRENT_DESCRIPTION") {
			t.Fatal("legacy file moved to the new task or current file was lost")
		}
		if persist {
			data, _ := json.Marshal(replay)
			if strings.Count(string(data), "TEVHQUNZ") != 1 || strings.Count(string(data), "Q1VSUkVOVA==") != 1 {
				t.Fatal("original inline bytes were lost/duplicated in replay")
			}
		} else if replay != nil {
			t.Fatal("store:false retained a persistence snapshot")
		}
	}
	_, originalFiles, ok := h.getResponseStore().continuationContext(owner, "legacy-response")
	if !ok || originalFiles["files"] == nil {
		t.Fatal("legacy migration mutated the stored parent record")
	}
}
