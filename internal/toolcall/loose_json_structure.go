package toolcall

import (
	"regexp"
	"strings"
)

var unquotedKeyPattern = regexp.MustCompile(`([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)

// All edits use offsets in a same-length structural mask. Quoted values,
// escapes and whitespace in the source are copied byte for byte.
func jsonStructureMask(raw string) (string, bool) {
	mask := []byte(raw)
	quoted, escaped := false, false
	for i, ch := range mask {
		if !quoted {
			quoted = ch == '"'
			continue
		}
		mask[i] = ' '
		if escaped {
			escaped = false
		} else if ch == '\\' {
			escaped = true
		} else if ch == '"' {
			mask[i], quoted = '"', false
		}
	}
	return string(mask), !quoted
}

func insertJSONStructure(raw string, edits map[int]string) string {
	if len(edits) == 0 {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw) + len(edits))
	for i := 0; i <= len(raw); i++ {
		b.WriteString(edits[i])
		if i < len(raw) {
			b.WriteByte(raw[i])
		}
	}
	return b.String()
}

func quoteLooseJSONKeys(raw string) string {
	mask, complete := jsonStructureMask(raw)
	if !complete {
		return raw
	}
	edits := map[int]string{}
	for _, match := range unquotedKeyPattern.FindAllStringSubmatchIndex(mask, -1) {
		edits[match[4]], edits[match[5]] = "\"", "\""
	}
	return insertJSONStructure(raw, edits)
}

func bracketLooseJSONObjectLists(raw string) string {
	mask, complete := jsonStructureMask(raw)
	if !complete {
		return raw
	}
	// Index balanced containers once, including arrays inside objects. This
	// avoids rescanning deeply nested parameter values for every colon.
	ends := map[int]int{}
	var stack []int
	for i := 0; i < len(mask); i++ {
		switch mask[i] {
		case '{', '[':
			stack = append(stack, i)
		case '}', ']':
			if len(stack) == 0 {
				return raw
			}
			start := stack[len(stack)-1]
			if mask[start] == '{' && mask[i] != '}' || mask[start] == '[' && mask[i] != ']' {
				return raw
			}
			ends[start] = i
			stack = stack[:len(stack)-1]
		}
	}
	edits := map[int]string{}
	for i := 0; i < len(mask); i++ {
		if mask[i] != ':' {
			continue
		}
		start := skipJSONSpace(mask, i+1)
		end, ok := ends[start]
		if !ok || mask[start] != '{' {
			continue
		}
		count := 1
		for next := skipJSONSpace(mask, end+1); next < len(mask) && mask[next] == ','; next = skipJSONSpace(mask, end+1) {
			object := skipJSONSpace(mask, next+1)
			last, found := ends[object]
			if !found || mask[object] != '{' {
				break
			}
			end, count = last, count+1
		}
		if count > 1 {
			edits[start], edits[end+1] = "[", "]"
		}
	}
	return insertJSONStructure(raw, edits)
}

func skipJSONSpace(s string, pos int) int {
	for pos < len(s) && strings.ContainsRune(" \t\r\n", rune(s[pos])) {
		pos++
	}
	return pos
}
