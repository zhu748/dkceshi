package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

type continuationFilesDS struct {
	responsesHistoryDS
	uploads int
}

func (d *continuationFilesDS) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	d.uploads++
	tokens := 9
	return &dsclient.UploadFileResult{ID: "file-attachment", Status: "SUCCESS", Bytes: int64(len(req.Data)), TokenUsage: &tokens}, nil
}

func (d *continuationFilesDS) FetchUploadedFile(_ context.Context, _ *auth.RequestAuth, id string) (*dsclient.UploadFileResult, error) {
	tokens := 9
	return &dsclient.UploadFileResult{ID: id, Status: "SUCCESS", TokenUsage: &tokens}, nil
}

func TestContinuationPreservesRequestWideFileInputs(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, files := range []string{
			`"file_ids":["file-original"]`,
			`"ref_file_ids":["file-original"]`,
			`"attachments":[{"file_id":"file-original"}]`,
			`"attachments":[{"type":"input_file","filename":"note.txt","file_data":"data:text/plain;base64,QUJDRA=="}]`,
			`"files":[{"file_id":"file-original","filename":"ORIGINAL.txt","description":"FILE_CONTEXT"}]`,
			`"files":[{"type":"input_file","filename":"note.txt","file_data":"data:text/plain;base64,QUJDRA==","description":"FILE_CONTEXT"}]`,
		} {
			t.Run(fmt.Sprintf("stream=%v/%s", stream, files), func(t *testing.T) {
				store, resolver := newDirectTokenResolver(t)
				ds := &continuationFilesDS{}
				h := &Handler{Store: store, Auth: resolver, DS: ds}
				send := func(body string) map[string]any {
					if stream {
						body = strings.TrimSuffix(body, "}") + `,"stream":true}`
					}
					req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
					req.Header.Set("Authorization", "Bearer direct-token")
					rec := httptest.NewRecorder()
					h.Responses(rec, req)
					if rec.Code != 200 {
						t.Fatalf("%d %s", rec.Code, rec.Body.String())
					}
					if !stream {
						return decodeJSONBody(t, rec.Body.String())
					}
					for _, line := range strings.Split(rec.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data: {") {
							continue
						}
						var event map[string]any
						if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
							t.Fatal(err)
						}
						if event["type"] == "response.completed" {
							return event["response"].(map[string]any)
						}
					}
					t.Fatalf("stream did not complete: %s", rec.Body.String())
					return nil
				}
				first := send(fmt.Sprintf(`{"model":"deepseek-v4.1-flash","input":"describe the file",%s}`, files))
				initial := append([]any(nil), ds.payload["ref_file_ids"].([]any)...)
				if len(initial) != 1 {
					t.Fatalf("initial references missing: %+v", ds.payload)
				}
				second := send(fmt.Sprintf(`{"model":"deepseek-v4.1-flash","previous_response_id":%q,"input":"explain more"}`, first["id"]))
				refs := ds.payload["ref_file_ids"].([]any)
				if len(refs) != 1 || refs[0] != initial[0] {
					t.Fatalf("file lost in continuation: before=%v after=%v", initial, refs)
				}
				if strings.Contains(files, "FILE_CONTEXT") && strings.Count(ds.payload["prompt"].(string), "FILE_CONTEXT") != 1 {
					t.Fatal("continuation lost or duplicated attachment metadata")
				}
				send(fmt.Sprintf(`{"model":"deepseek-v4.1-flash","previous_response_id":%q,"input":"continue","file_ids":["file-new","file-original"]}`, second["id"]))
				refs = ds.payload["ref_file_ids"].([]any)
				if len(refs) < 2 {
					t.Fatalf("old/new file merge lost references: %v", refs)
				}
			})
		}
	}

}

func TestStoredFileInputsAreIsolatedAndCountTowardCapacity(t *testing.T) {
	s := newResponseStore(0)
	files := map[string]any{"attachments": []any{map[string]any{"file_id": "file-original"}}}
	context := []any{map[string]any{"role": "user", "content": "describe"}}
	if !s.putContext("owner", "response", map[string]any{"id": "response"}, context, files) {
		t.Fatal("store failed")
	}
	files["attachments"].([]any)[0].(map[string]any)["file_id"] = "mutated"
	_, restored, ok := s.continuationContext("owner", "response")
	if !ok || restored["attachments"].([]any)[0].(map[string]any)["file_id"] != "file-original" {
		t.Fatal("stored files alias caller input")
	}
	if _, _, ok := s.continuationContext("other", "response"); ok {
		t.Fatal("cross-caller files exposed")
	}
	s.limits.bytes = 100
	if s.putContext("owner", "large", map[string]any{}, context, map[string]any{"attachments": strings.Repeat("x", 200)}) {
		t.Fatal("file payload bypassed cache capacity")
	}
}

func TestReplayFileIDsRemainFlatAndDeduplicated(t *testing.T) {
	var previous map[string]any
	for range 100 {
		req := map[string]any{"file_ids": []any{"file-original", "file-original"}}
		mergeReplayFiles(req, previous)
		ids := req["file_ids"].([]string)
		if len(ids) != 1 || ids[0] != "file-original" {
			t.Fatalf("repeated continuation expanded IDs: %v", ids)
		}
		previous = map[string]any(captureReplayFiles(req))
	}
	req := map[string]any{"store": false}
	mergeReplayFiles(req, previous)
	if _, ok := req[replayFilesKey]; ok {
		t.Fatal("store:false cloned files for persistence")
	}
}
