package requestbody

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestBodyValidationRejectsEscapedInvalidUnicodeBeforeReplay(t *testing.T) {
	for _, raw := range []string{`{"text":"\ud800"}`, `{"text":"\udc00"}`, `{"\udfff":"x"}`} {
		body := validateAndReplayBody(io.NopCloser(strings.NewReader(raw)), 4096)
		_, err := io.ReadAll(body)
		if closeErr := body.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if !errors.Is(err, ErrInvalidJSONBody) {
			t.Fatalf("invalid escape reached JSON decoder: %s, %v", raw, err)
		}
	}
}
