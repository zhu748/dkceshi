package completionruntime

import (
	"context"
	"net/http"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
)

func resumeStream(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, sessionID string, messageID int, pow string) (*http.Response, *assistantturn.OutputError) {
	resumer, ok := ds.(continueResumer)
	if !ok {
		return nil, &assistantturn.OutputError{Status: http.StatusInternalServerError, Message: "Streaming resume via continue endpoint is not supported by this caller.", Code: "error"}
	}
	resp, err := resumer.ContinueCompletion(ctx, a, sessionID, messageID, pow)
	if err != nil {
		return nil, &assistantturn.OutputError{Status: http.StatusBadGateway, Message: "Failed to resume interrupted stream: " + err.Error(), Code: "upstream_interrupted"}
	}
	if outErr := completionResponseError(resp, a); outErr != nil {
		// The client already has partial output. Never start a fresh generation
		// on another account when a continuation fails.
		return nil, &assistantturn.OutputError{Status: http.StatusBadGateway, Message: outErr.Message, Code: "upstream_interrupted"}
	}
	return resp, nil
}
