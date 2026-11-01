package promptcompat

// Settings remain server-owned; request fields cannot weaken tool recovery.
func ApplyToolSafetySettings(store any, policy ToolChoicePolicy) ToolChoicePolicy {
	if reader, ok := store.(interface{ SettingBool(string, string) bool }); ok {
		policy.AllowReasoningProse = reader.SettingBool("tool_safety", "allow_reasoning_prose")
	}
	return policy
}
