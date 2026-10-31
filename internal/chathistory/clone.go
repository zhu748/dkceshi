package chathistory

import (
	"encoding/json"
)

func cloneFile(in File) File {
	out := File{
		Version:  in.Version,
		Limit:    in.Limit,
		Revision: in.Revision,
		Items:    make([]SummaryEntry, len(in.Items)),
	}
	copy(out.Items, in.Items)
	return out
}

func cloneEntry(item Entry) Entry {
	item.Usage = cloneMap(item.Usage)
	item.Messages = cloneMessages(item.Messages)
	item.Tools = append(json.RawMessage(nil), item.Tools...)
	item.ToolChoice = append(json.RawMessage(nil), item.ToolChoice...)
	return item
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

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
