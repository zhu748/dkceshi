package history

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

type historyThinkingConfig struct {
	enabled bool
	prompt  string
}

func (c historyThinkingConfig) ThinkingInjectionEnabled() bool  { return c.enabled }
func (c historyThinkingConfig) ThinkingInjectionPrompt() string { return c.prompt }

func TestHistoryFileStartsWithEffectiveThinkingInstruction(t *testing.T) {
	for _, tc := range []struct {
		name, prompt      string
		enabled, thinking bool
	}{
		{name: "default", enabled: true, thinking: true},
		{name: "custom", enabled: true, thinking: true, prompt: "Custom reasoning.\nPrior conversation history and tool progress."},
		{name: "disabled", thinking: true},
		{name: "without_thinking", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := markerStandardRequest(markerUnderTest)
			req.Thinking = tc.thinking
			req = promptcompat.ApplyThinkingInjection(historyThinkingConfig{enabled: tc.enabled, prompt: tc.prompt}, req)
			uploader := &recordingUploader{}
			out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
			if err != nil {
				t.Fatal(err)
			}
			uploaded, ok := uploader.uploadFor(currentInputFilename)
			if !ok {
				t.Fatal("missing history upload")
			}
			prefix := ""
			if tc.enabled && tc.thinking {
				prompt := tc.prompt
				if prompt == "" {
					prompt = promptcompat.DefaultThinkingInjectionPrompt
				}
				prefix = "<｜System｜>" + prompt + "\n\n"
			}
			wantStart := prefix + "# HISTORY.txt\nPrior conversation history and tool progress.\n" + currentInputContinuationInstruction
			if !strings.HasPrefix(string(uploaded.Data), wantStart) {
				t.Fatalf("wrong history instruction order: %q", uploaded.Data)
			}
			if out.HistoryText != string(uploaded.Data) {
				t.Fatal("archived history differs from the uploaded file")
			}
			if prefix != "" && !strings.HasPrefix(out.FinalPrompt, prefix) {
				t.Fatal("live continuation lost the thinking instruction")
			}
			tools, _ := uploader.uploadFor(currentToolsFilename)
			wantTokens := out.HistoryText + "\n" + string(tools.Data) + "\n" + out.FinalPrompt
			if out.PromptTokenText != wantTokens {
				t.Fatal("token text does not match the full uploaded and live context")
			}
		})
	}
}

func TestHistoryThinkingPrefixSurvivesReuploadWithoutDuplication(t *testing.T) {
	req := markerStandardRequest(markerUnderTest)
	req.Thinking = true
	req = promptcompat.ApplyThinkingInjection(historyThinkingConfig{enabled: true}, req)
	first := &recordingUploader{}
	out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: first}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.WithHistoryThinkingInstruction(out.HistoryText); got != out.HistoryText {
		t.Fatal("reapplying the prefix duplicated it")
	}
	second := &recordingUploader{}
	reuploaded, err := (Service{DS: second}).ReuploadAppliedCurrentInputFile(context.Background(), &auth.RequestAuth{}, out)
	if err != nil {
		t.Fatal(err)
	}
	uploaded, ok := second.uploadFor(currentInputFilename)
	if !ok || string(uploaded.Data) != out.HistoryText || reuploaded.HistoryText != out.HistoryText {
		t.Fatal("account failover changed the prefixed history bytes")
	}
	if strings.Count(string(uploaded.Data), promptcompat.ThinkingInjectionMarker) != 1 {
		t.Fatal("thinking instruction was dropped or duplicated during reupload")
	}
}
