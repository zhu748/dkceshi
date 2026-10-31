package assistantturn

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestPromptEchoIsNotAnAnswer(t *testing.T) {
	prefix := "<｜System｜>" + promptcompat.DefaultThinkingInjectionPrompt
	prompt := prefix + "\n\nSerialized conversation. Each message ends with [EndMessage:0123456789abcdef].\n[User]:continue\n[Assistant]:"
	// The exported failure paraphrased a later sentence, but copied the
	// generated first line and serializer header before the terminator cut.
	echo := "\n\n" + strings.Replace(prefix, "and are certain", "and be certain", 1) + "\n\nSerialized conversation. Each message ends with "
	opts := BuildOptions{Prompt: prompt}
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: echo, Thinking: "I will make two edits."}, opts),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: echo, VisibleText: echo, RawThinking: "I will make two edits.", VisibleThinking: "I will make two edits."}, opts),
	} {
		if turn.Error == nil || turn.Error.Code != "upstream_prompt_echo" || turn.Error.Status != 502 || turn.Text != "" || len(turn.ToolCalls) != 0 {
			t.Fatalf("prompt echo accepted: text=%q error=%+v calls=%v", turn.Text, turn.Error, turn.ToolCalls)
		}
		if turn.Thinking != "I will make two edits." || !ShouldRetryEmptyOutput(turn, 0, 1) {
			t.Fatalf("reasoning or retry contract changed: %+v", turn)
		}
	}
}

func TestPromptEchoCannotRecoverExecutableCallsFromReasoning(t *testing.T) {
	const prefix = "<｜System｜>custom thinking instruction"
	const plan = `<tool_calls><invoke name="read"><parameter name="path">plan.txt</parameter></invoke></tool_calls>`
	opts := BuildOptions{Prompt: prefix, ToolNames: []string{"read"}, ToolCallRepair: func(context.Context, string) (string, error) {
		t.Fatal("prompt echo must not enter tool-call repair")
		return "", nil
	}}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: prefix, Thinking: plan}, opts)
	if turn.Error == nil || turn.Error.Code != OutputCodePromptEcho || len(turn.ToolCalls) != 0 || len(turn.ParsedToolCalls.Calls) != 0 {
		t.Fatalf("rejected answer recovered a tool from reasoning: %+v", turn)
	}
}

func TestPromptEchoGuardPreservesQuotedAndOrdinaryAnswers(t *testing.T) {
	line := "<｜System｜>custom thinking instruction"
	for _, text := range []string{"正常回答。", "The input begins with " + line, "`" + line + "`", "```text\n" + line + "\n```", "\"" + line + "\"", "<｜System｜>a different literal instruction"} {
		turn := BuildTurnFromCollected(sse.CollectResult{Text: text}, BuildOptions{Prompt: line + "\n\n[User]:hello"})
		if turn.Text != text || turn.Error != nil {
			t.Fatalf("legitimate text changed: input=%q got=%q err=%+v", text, turn.Text, turn.Error)
		}
	}
}
