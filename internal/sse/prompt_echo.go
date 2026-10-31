package sse

import "strings"

// PromptEchoGuard only examines the start of the answer. Its fingerprints come
// from the generated prelude actually sent in this request, not reply keywords.
// Code, quotes, later mentions and different instructions pass through intact.
type PromptEchoGuard struct {
	prefixes []string
	pending  string
	done     bool
	tripped  bool
}

const maxEchoPrefixBytes = 256

func NewPromptEchoGuard(prompt string) PromptEchoGuard {
	var prefixes []string
	text := strings.TrimSpace(prompt)
	if strings.HasPrefix(text, "<｜System｜>") {
		line, _, _ := strings.Cut(text, "\n")
		prefixes = append(prefixes, line[:min(len(line), maxEchoPrefixBytes)])
	}
	const transcriptIntro = "Serialized conversation. Each message ends with "
	if strings.Contains(prompt, transcriptIntro+"[EndMessage:") {
		prefixes = append(prefixes, transcriptIntro)
	}
	return PromptEchoGuard{prefixes: prefixes, done: len(prefixes) == 0}
}

func (g *PromptEchoGuard) Feed(text string) string {
	if g.tripped {
		return ""
	}
	if g.done {
		return text
	}
	data := g.pending + text
	start := strings.TrimLeft(data, " \r\n\t")
	for _, prefix := range g.prefixes {
		if strings.HasPrefix(start, prefix) {
			g.pending, g.tripped = "", true
			return ""
		}
	}
	if len(data) <= maxEchoPrefixBytes*2 {
		for _, prefix := range g.prefixes {
			if strings.HasPrefix(prefix, start) {
				g.pending = data
				return ""
			}
		}
	}
	g.pending, g.done = "", true
	return data
}

// Flush releases an unconfirmed prefix at EOF. It must not disappear merely
// because the legitimate answer ended before it diverged from a fingerprint.
func (g *PromptEchoGuard) Flush() string {
	text := g.pending
	g.pending, g.done = "", true
	return text
}

func IsPromptEcho(text, prompt string) bool {
	guard := NewPromptEchoGuard(prompt)
	guard.Feed(text)
	return guard.tripped
}
