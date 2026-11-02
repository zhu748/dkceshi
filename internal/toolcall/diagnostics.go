package toolcall

func appendRecovery(sinks []*[]string, reason string) {
	for _, sink := range sinks {
		if sink != nil {
			*sink = append(*sink, reason)
		}
	}
}

func syntaxRecoverySteps(original, normalized string, calls []ParsedToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	var steps []string
	if repairMissingXMLToolCallsOpeningWrapper(normalized) != normalized {
		steps = append(steps, "missing_opening_wrapper")
	}
	for pos := 0; pos < len(original); {
		tag, ok := FindToolMarkupTagOutsideIgnored(original, pos)
		if !ok {
			break
		}
		if tag.ParameterName != "" {
			steps = append(steps, "parameter_shorthand")
			break
		}
		pos = tag.End + 1
	}
	return steps
}

// DiagnosticToolNames preserves source occurrence order, including damaged
// siblings. Names are extracted from markup identities, never inferred from a
// catalog or from quoted examples.
func DiagnosticToolNames(source string) []string {
	var names []string
	for _, node := range repairIdentities(source) {
		if node.kind == "invoke" {
			names = append(names, node.name)
		}
	}
	return names
}
