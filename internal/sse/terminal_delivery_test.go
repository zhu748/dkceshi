package sse

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPumpDeliversTerminalContentBeforeStop(t *testing.T) {
	for _, accumulate := range []bool{false, true} {
		for _, thinkingTail := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{false: "direct", true: "accumulated"}[accumulate], map[bool]string{false: "batch", true: "thinking_tail"}[thinkingTail]}, "/"), func(t *testing.T) {
				body := "data: {\"p\":\"response\",\"o\":\"APPEND\",\"v\":[{\"p\":\"content\",\"v\":\"final answer\"},{\"p\":\"status\",\"v\":\"FINISHED\"}]}\n"
				want, kind := "final answer", "text"
				if thinkingTail {
					body = "data: {\"p\":\"response/thinking_content\",\"v\":\"analysis<\"}\n" +
						"data: {\"p\":\"response/status\",\"v\":\"FINISHED\"}\n"
					want, kind = "analysis<", "thinking"
				}
				cfg := productionAccumulate
				cfg.Enabled = accumulate
				results, done := startParsedLinePumpWithConfig(context.Background(), strings.NewReader(body), thinkingTail, kind, cfg)
				var got strings.Builder
				stopped := false
				for frame := range results {
					for _, part := range frame.Parts {
						if stopped {
							t.Error("content delivered after terminal frame")
						}
						got.WriteString(part.Text)
					}
					stopped = stopped || frame.Stop
				}
				if err := <-done; err != nil || !stopped || got.String() != want {
					t.Fatalf("got %q stopped=%v err=%v; want %q", got.String(), stopped, err, want)
				}
			})
		}
	}
}

func TestDirectPumpPreservesResponseMessageIDWithoutContent(t *testing.T) {
	cfg := productionAccumulate
	cfg.Enabled = false
	results, done := startParsedLinePumpWithConfig(context.Background(), strings.NewReader("data: {\"response_message_id\":42}\n"), false, "text", cfg)
	seen := false
	for frame := range results {
		seen = seen || frame.ResponseMessageID == 42
	}
	if err := <-done; !errors.Is(err, ErrUnexpectedStreamEOF) || !seen {
		t.Fatalf("message ID lost: seen=%v err=%v", seen, err)
	}
}
