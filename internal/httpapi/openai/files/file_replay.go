package files

import (
	"context"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

const replaySnapshotKey = "_inline_replay_snapshot"

// A private type prevents client JSON from injecting executable replay state.
type inlineReplaySnapshot struct {
	source   map[string]any
	uploader FileUploader
	model    string
	thinking bool
	owner    string
}

// WithReplay keeps protocol normalization at the adapter boundary. Replaying
// bytes rebuilds the entire normalized request, never edits IDs inside prose.
func WithReplay(standard promptcompat.StandardRequest, a *auth.RequestAuth, req map[string]any, normalize func(context.Context, map[string]any) (promptcompat.StandardRequest, error)) promptcompat.StandardRequest {
	snapshot, ok := req[replaySnapshotKey].(*inlineReplaySnapshot)
	if !ok {
		return standard
	}
	base := standard
	standard.InputAccountID = snapshot.owner
	standard.ReplayInput = func(ctx context.Context) (promptcompat.StandardRequest, error) {
		working := cloneFileInput(snapshot.source).(map[string]any)
		if err := PreprocessInlineFiles(ctx, a, working, snapshot.uploader, snapshot.model, snapshot.thinking); err != nil {
			return promptcompat.StandardRequest{}, err
		}
		normalized, err := normalize(ctx, working)
		if err != nil {
			return normalized, err
		}
		// Settings may change while a stream is running. Preserve this request's
		// model, tool policy and generated instructions when replacing its input.
		out := base
		out.Messages, out.RefFileIDs, out.RefFileTokens = normalized.Messages, normalized.RefFileIDs, normalized.RefFileTokens
		out.FinalPrompt, out.ToolNames = out.BuildPromptFromMessages(out.Messages)
		out.PromptTokenText = out.FinalPrompt
		return WithReplay(out, a, working, normalize), nil
	}
	return standard
}
