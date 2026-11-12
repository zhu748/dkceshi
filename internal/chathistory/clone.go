package chathistory

import (
	"ds2api/internal/tokenusage"
	"encoding/json"
)

func cloneFile(in File) File {
	out := File{
		Version:  in.Version,
		Limit:    in.Limit,
		Revision: in.Revision,
		Items:    make([]SummaryEntry, len(in.Items)),
	}
	for i, item := range in.Items {
		out.Items[i] = item
		if item.Usage != nil {
			raw := *item.Usage
			raw.PromptTokens = copyCount(raw.PromptTokens)
			raw.CompletionTokens = copyCount(raw.CompletionTokens)
			raw.ReasoningTokens = copyCount(raw.ReasoningTokens)
			raw.TotalTokens = copyCount(raw.TotalTokens)
			raw.AccumulatedTokens = copyCount(raw.AccumulatedTokens)
			if raw.Complete != nil {
				complete := *raw.Complete
				raw.Complete = &complete
			}
			out.Items[i].Usage = &raw
		}
	}
	return out
}

func cloneEntry(item Entry) Entry {
	item.Usage = cloneMap(item.Usage)
	item.Messages = cloneMessages(item.Messages)
	item.Tools = append(json.RawMessage(nil), item.Tools...)
	item.ToolChoice = append(json.RawMessage(nil), item.ToolChoice...)
	item.ContextManifest = append(json.RawMessage(nil), item.ContextManifest...)
	return item
}

func cloneMap(in map[string]any) map[string]any { return tokenusage.CloneMap(in) }

func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return []Message{}
	}
	out := make([]Message, len(messages))
	copy(out, messages)
	for i := range out {
		out[i].ToolCalls = append(json.RawMessage(nil), out[i].ToolCalls...)
		out[i].FunctionCall = append(json.RawMessage(nil), out[i].FunctionCall...)
		out[i].ReasoningContent = append(json.RawMessage(nil), out[i].ReasoningContent...)
		if out[i].IsError != nil {
			value := *out[i].IsError
			out[i].IsError = &value
		}
		if out[i].Prefix != nil {
			value := *out[i].Prefix
			out[i].Prefix = &value
		}
	}
	return out
}

func copyCount(n *int) *int {
	if n == nil {
		return nil
	}
	copy := *n
	return &copy
}
