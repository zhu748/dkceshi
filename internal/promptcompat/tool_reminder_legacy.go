package promptcompat

import "strings"

// Settings forms may persist the built-in text rather than an empty override.
// Recognize only exact published defaults; genuinely custom prompts are kept.
const legacyBuiltinToolReminder = `The only correct format for using tools is <|EPSE|tool_calls>
  <|EPSE|invoke name="TOOL_NAME_HERE">
    <|EPSE|parameter name="PARAMETER_NAME"><![CDATA[PARAMETER_VALUE]]></|EPSE|parameter>
  </|EPSE|invoke>
</|EPSE|tool_calls> .Please do not simplify or modify this framework in any way.Use exactly the tag prefix shown above; do not substitute another tool format.`

func isLegacyBuiltinToolReminder(prompt string) bool {
	prompt = strings.ReplaceAll(strings.TrimSpace(prompt), "\r\n", "\n")
	if prompt == legacyBuiltinToolReminder {
		return true
	}
	upstreamDefault := strings.Replace(legacyBuiltinToolReminder,
		"Use exactly the tag prefix shown above; do not substitute another tool format.",
		"The only valid label is EPSE; the use of any label prefixed with DSML is **strictly prohibited**.", 1)
	return prompt == upstreamDefault || prompt == "For client-provided tools only:\n"+upstreamDefault
}
