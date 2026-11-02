package tooldiagnostics

import "unicode/utf8"

const MaxRecordTextBytes = 4 << 20

// Limit the aggregate evidence, not just individual snapshots. Preserve the
// newest attempt first, original byte counts and all structural/result fields.
// Clone slices so serialization never mutates a live recorder's evidence.
func boundRecord(record Record) Record {
	remaining := MaxRecordTextBytes
	take := func(value Text) Text {
		if len(value.Text) <= remaining {
			remaining -= len(value.Text)
			return value
		}
		record.EvidenceTruncated = true
		head, tail := remaining/2, len(value.Text)-remaining/2
		for head > 0 && !utf8.RuneStart(value.Text[head]) {
			head--
		}
		for tail < len(value.Text) && !utf8.RuneStart(value.Text[tail]) {
			tail++
		}
		value.Text = value.Text[:head] + value.Text[tail:]
		value.Truncated = true
		remaining = 0
		return value
	}
	clip := func(text string) string {
		if len(text) > 8192 {
			text = Capture(text).Text
			end := 8192
			for end > 0 && !utf8.RuneStart(text[end]) {
				end--
			}
			text = text[:end] + "…"
			record.EvidenceTruncated = true
		}
		return take(Text{Text: text, Bytes: len(text)}).Text
	}
	record.Error = clip(record.Error)
	record.Attempts = append([]Attempt(nil), record.Attempts...)
	for i := len(record.Attempts) - 1; i >= 0; i-- {
		a := &record.Attempts[i]
		a.Error, a.RetryReason = clip(a.Error), clip(a.RetryReason)
		a.Raw, a.Residual = take(a.Raw), take(a.Residual)
		a.Calls = append([]Call{}, a.Calls...)
		for j := range a.Calls {
			a.Calls[j].Error = clip(a.Calls[j].Error)
			a.Calls[j].Arguments = take(a.Calls[j].Arguments)
		}
		a.Steps = append([]Step{}, a.Steps...)
		for j := len(a.Steps) - 1; j >= 0; j-- {
			a.Steps[j].Error, a.Steps[j].Reason = clip(a.Steps[j].Error), clip(a.Steps[j].Reason)
			a.Steps[j].Input, a.Steps[j].Output = take(a.Steps[j].Input), take(a.Steps[j].Output)
		}
	}
	record.Tools, record.Policy = take(record.Tools), take(record.Policy)
	return record
}
