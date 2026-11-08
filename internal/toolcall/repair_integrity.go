package toolcall

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errRepairIntegrity = errors.New("repair integrity check failed")
var errIncompleteRepair = errors.New("repair output is incomplete")

func repairParametersKnown(source cdataSlotResult, marker string) bool {
	for _, node := range repairIdentities(closeRepairManifestCDATA(NormalizeMarkerText(source.skeleton, marker))) {
		if node.kind == "unbound" || node.kind == "parameter" && !node.known {
			return false
		}
	}
	return true
}

// A format repair may fix shells, but must preserve the ordered tool/parameter
// identities and every value whose source boundary is known. This also binds
// each CDATA placeholder to its original parameter, before restoring its bytes.
type repairIdentity struct {
	kind, name string
	value      any
	known      bool
}

func validateRepairIntegrity(source cdataSlotResult, output, marker string) error {
	// Close only the CDATA shells in the comparison copy. In particular the
	// greedy alternate must not hide an unclosed parameter that primary saw.
	skeleton := closeRepairManifestCDATA(NormalizeMarkerText(source.skeleton, marker))
	expected := repairIdentities(skeleton)
	actual := repairIdentities(output)
	if len(expected) != len(actual) {
		return fmt.Errorf("%w: changed the number of named tool/parameter nodes", errRepairIntegrity)
	}
	for i, node := range expected {
		got := actual[i]
		if node.kind == "unbound" || node.kind == "parameter" && !node.known {
			return fmt.Errorf("%w: source parameter boundary is unknown at node %d", errRepairIntegrity, i)
		}
		if node.kind != got.kind || node.name != got.name {
			return fmt.Errorf("%w: changed tool/parameter identity or order at node %d", errRepairIntegrity, i)
		}
		if node.known && (!got.known || !reflect.DeepEqual(node.value, got.value)) {
			return fmt.Errorf("%w: changed a source parameter value at node %d", errRepairIntegrity, i)
		}
	}
	return nil
}

// HasNamedToolInvocation distinguishes broken client calls from orphan shells,
// ordinary prose. Empty names accepts any named invocation, including a
// schema-less continuation; otherwise names filters exact tool names. Literal
// code/CDATA is skipped by the same scanner used for repair integrity.
func HasNamedToolInvocation(text string, names []string) bool {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	for _, node := range repairIdentities(stripFencedCodeBlocks(closeRepairManifestCDATA(text))) {
		if node.kind == "invoke" && (len(names) == 0 || allowed[node.name]) {
			return true
		}
	}
	return false
}

func closeRepairManifestCDATA(text string) string {
	var b strings.Builder
	prev := 0
	for _, r := range scanBoundedCDATARanges(text) {
		if toolCDATACloseLenAt(text, r.contentEnd) == 0 {
			b.WriteString(text[prev:r.contentEnd])
			b.WriteString("]]>")
			prev = r.contentEnd + r.closeLen
		}
	}
	if prev == 0 {
		return text
	}
	b.WriteString(text[prev:])
	return b.String()
}

func repairIdentities(text string) []repairIdentity {
	var nodes []repairIdentity
	inInvoke := false
	inBatch := false
	var batchHasCall []bool
	unbound := func() {
		if len(nodes) == 0 || nodes[len(nodes)-1].kind != "unbound" {
			nodes = append(nodes, repairIdentity{kind: "unbound"})
		}
	}
	for i := 0; i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			if inInvoke || inBatch {
				unbound()
			}
			break
		}
		if advanced {
			if (inInvoke || inBatch) && !emptyToolMarkupGap(text[i:next]) {
				unbound()
			}
			i = next
			continue
		}
		if end, ok := markdownCodeSpanEnd(text, i); ok {
			if inInvoke || inBatch {
				unbound()
			}
			i = end
			continue
		}
		kind, end, closing := "", -1, false
		invocationTag := false
		parameterName := ""
		closeStart, closeEnd := -1, -1
		if tag, ok := scanToolMarkupTagAt(text, i); ok {
			kind, end, closing = tag.Name, tag.End, tag.Closing
			invocationTag = tag.Name == "invoke"
			parameterName = tag.ParameterName
			if kind == "parameter" && tag.NameStart == tag.NameEnd {
				// The lenient markup scanner also calls unknown named EPSE
				// shells shorthand parameters. A nonempty local name such as
				// "call" identifies the invocation being repaired instead.
				tail := text[tag.NameStart : tag.End+1]
				if attr := xmlAttrPattern.FindStringIndex(tail); attr != nil && strings.IndexFunc(tail[:attr[0]], unicode.IsLetter) >= 0 {
					kind = "invoke"
				}
			}
			if kind == "parameter" && !closing {
				if close, ok := FindMatchingToolMarkupClose(text, tag); ok {
					closeStart, closeEnd = close.Start, close.End
				}
			}
		} else if tag, ok := scanDSMLTagAt(text, i); ok {
			kind, end, closing = tag.Name, tag.End, tag.Closing
			invocationTag = tag.Name == "invoke"
			if kind == "parameter" && !closing {
				if close, ok := FindMatchingDSMLClose(text, tag); ok {
					closeStart, closeEnd = close.Start, close.End
				}
			}
		} else if after, isClose, ok := detectEPSETagAt(text, i); ok {
			// Wrong local names (notably "call") are exactly what the repair
			// fixes. Their declared tool name is still immutable.
			kind, end, closing = "invoke", findXMLTagEnd(text, after), isClose
		}
		if end < i {
			r, size := utf8.DecodeRuneInString(text[i:])
			if ((inInvoke || inBatch) && !unicode.IsSpace(r)) || kind != "" && !closing || unfinishedRepairShellAt(text, i) {
				unbound()
			}
			i += max(size, nextToolMarkupProbe(text, i)-i)
			continue
		}
		attrs := parseXMLTagAttributes(text[i : end+1])
		if attrs == nil {
			unbound()
		}
		name := html.UnescapeString(attrs["name"])
		if kind != "parameter" {
			name = strings.TrimSpace(name)
		} else if _, present := attrs["name"]; !closing && !present && parameterName == "" {
			unbound()
		}
		if parameterName != "" {
			name = parameterName
		}
		if invocationTag && !closing && name == "" {
			unbound()
		}
		jsonEnd := -1
		if kind == "invoke" {
			inInvoke = !closing
		}
		if kind == "tool_calls" || kind == "calls" {
			if !closing {
				batchHasCall = append(batchHasCall, false)
			} else if n := len(batchHasCall); n > 0 {
				if !batchHasCall[n-1] {
					unbound()
				}
				batchHasCall = batchHasCall[:n-1]
			}
			inBatch = len(batchHasCall) > 0
		}
		if !closing && (name != "" || kind == "parameter") {
			if kind != "parameter" {
				kind = "invoke"
			}
			node := repairIdentity{kind: kind, name: name}
			if kind == "invoke" {
				for j := range batchHasCall {
					batchHasCall[j] = true
				}
				raw := strings.TrimLeftFunc(text[end+1:], unicode.IsSpace)
				if strings.HasPrefix(raw, "{") {
					dec := json.NewDecoder(strings.NewReader(raw))
					dec.UseNumber()
					var value map[string]any
					if dec.Decode(&value) == nil {
						node.value, node.known = value, true
						if conflictingJSONKeys(raw[:dec.InputOffset()]) {
							node.kind, node.known = "unbound", false
						}
						jsonEnd = end + 1 + len(text[end+1:]) - len(raw) + int(dec.InputOffset())
					}
				}
			}
			if kind == "parameter" && closeStart >= 0 {
				raw := text[end+1 : closeStart]
				if literal, ok := extractStandaloneCDATA(strings.TrimSpace(raw)); ok {
					node.value = literal
				} else {
					node.value = parseInvokeParameterValue(name, raw)
				}
				node.known = true
				if conflictingJSONKeys(text[end+1 : closeStart]) {
					node.kind, node.known = "unbound", false
				}
			}
			nodes = append(nodes, node)
		}
		if closeEnd >= 0 {
			i = closeEnd + 1
		} else if jsonEnd > end {
			i = jsonEnd
		} else {
			i = end + 1
		}
	}
	for _, hasCall := range batchHasCall {
		if !hasCall {
			unbound()
			break
		}
	}
	return nodes
}
