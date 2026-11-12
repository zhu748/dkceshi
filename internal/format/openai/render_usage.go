package openai

import "ds2api/internal/assistantturn"

func BuildChatUsageForModel(model, finalPrompt, finalThinking, finalText string, refFileTokens int) map[string]any {
	return assistantturn.OpenAIChatUsage(assistantturn.Turn{Usage: assistantturn.BuildUsage(model, finalPrompt, finalThinking, finalText, refFileTokens)})
}
func BuildChatUsage(finalPrompt, finalThinking, finalText string) map[string]any {
	return BuildChatUsageForModel("", finalPrompt, finalThinking, finalText, 0)
}
func BuildResponsesUsageForModel(model, finalPrompt, finalThinking, finalText string, refFileTokens int) map[string]any {
	return assistantturn.OpenAIResponsesUsage(assistantturn.Turn{Usage: assistantturn.BuildUsage(model, finalPrompt, finalThinking, finalText, refFileTokens)})
}
func BuildResponsesUsage(finalPrompt, finalThinking, finalText string) map[string]any {
	return BuildResponsesUsageForModel("", finalPrompt, finalThinking, finalText, 0)
}
