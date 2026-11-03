package contextfile

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

const (
	currentInputFilename    = promptcompat.CurrentInputContextFilename
	currentToolsFilename    = promptcompat.CurrentToolsContextFilename
	currentInputContentType = "text/plain; charset=utf-8"
	currentInputPurpose     = "assistants"

	currentInputContinuationInstruction = "Continue from the latest state in the attached HISTORY.txt context. Treat it as the current working state and answer the latest user request directly.Do not mention HISTORY.txt in the main text."
)

type CurrentInputConfigReader interface {
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
}

type CurrentInputUploader interface {
	UploadFile(ctx context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, maxAttempts int) (*dsclient.UploadFileResult, error)
}

type Service struct {
	Store CurrentInputConfigReader
	DS    CurrentInputUploader
}

func (s Service) ApplyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if stdReq.CurrentInputFileApplied || s.DS == nil || s.Store == nil || a == nil || !s.Store.CurrentInputFileEnabled() {
		return stdReq, nil
	}
	if modelType, ok := config.GetModelType(stdReq.ResolvedModel); ok && modelType == "expert" {
		return stdReq, nil
	}
	if strings.TrimSpace(stdReq.FinalPrompt) == "" || utf8.RuneCountInString(stdReq.FinalPrompt) < s.Store.CurrentInputFileMinChars() {
		return stdReq, nil
	}
	if stdReq.ToolMarker == "" {
		stdReq.ToolMarker = a.ToolMarker
	}
	plan := planCurrentInputContext(stdReq)
	if plan == nil {
		return stdReq, nil
	}
	generated := 1
	if plan.toolsText != "" {
		generated++
	}
	if len(prependUniqueRefFileIDs(stdReq.RefFileIDs))+generated > promptcompat.MaxReferenceFiles {
		return stdReq, errContextFileReferenceLimit
	}
	fileID, err := s.uploadContextText(ctx, a, stdReq, currentInputFilename, plan.historyText)
	if err != nil {
		return stdReq, err
	}
	toolFileID, err := s.uploadContextText(ctx, a, stdReq, currentToolsFilename, plan.toolsText)
	if err != nil {
		return stdReq, err
	}
	stdReq.Messages = plan.messages
	stdReq.HistoryText = plan.historyText
	stdReq.FinalPrompt = plan.prompt
	stdReq.ToolNames = plan.toolNames
	stdReq.CurrentInputFileApplied = true
	stdReq.CurrentInputFileID = fileID
	stdReq.CurrentToolsFileID = toolFileID
	stdReq.RefFileIDs = prependUniqueRefFileIDs(stdReq.RefFileIDs, fileID, toolFileID)
	parts := []string{plan.historyText}
	if plan.toolsText != "" {
		parts = append(parts, plan.toolsText)
	}
	stdReq.PromptTokenText = strings.Join(append(parts, plan.prompt), "\n")
	return stdReq, nil
}

func (s Service) ReuploadAppliedCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if !stdReq.CurrentInputFileApplied || s.DS == nil || a == nil || strings.TrimSpace(stdReq.HistoryText) == "" {
		return stdReq, nil
	}
	if stdReq.ToolMarker == "" {
		stdReq.ToolMarker = a.ToolMarker
	}
	toolsText, _ := promptcompat.BuildOpenAIToolsContextTranscript(stdReq.ToolsRaw, stdReq.ToolChoice)
	if len(stdReq.RefFileIDs) > promptcompat.MaxReferenceFiles {
		return stdReq, errContextFileReferenceLimit
	}
	fileID, err := s.uploadContextText(ctx, a, stdReq, currentInputFilename, stdReq.HistoryText)
	if err != nil {
		return stdReq, err
	}
	toolFileID, err := s.uploadContextText(ctx, a, stdReq, currentToolsFilename, toolsText)
	if err != nil {
		return stdReq, err
	}
	stdReq.RefFileIDs = replaceGeneratedCurrentInputRefs(stdReq.RefFileIDs, stdReq.CurrentInputFileID, stdReq.CurrentToolsFileID, fileID, toolFileID)
	stdReq.CurrentInputFileID = fileID
	stdReq.CurrentToolsFileID = toolFileID
	return stdReq, nil
}

func (s Service) uploadContextText(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest, filename, content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", nil
	}
	modelType := "default"
	if resolved, ok := config.GetModelType(req.ResolvedModel); ok {
		modelType = resolved
	}
	result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
		Filename: filename, ContentType: currentInputContentType, Purpose: currentInputPurpose,
		ModelType: modelType, ThinkingEnabled: req.Thinking, Data: []byte(content),
	}, 3)
	if err != nil {
		return "", fmt.Errorf("upload context file %s: %w", filename, err)
	}
	if result == nil || strings.TrimSpace(result.ID) == "" {
		return "", fmt.Errorf("upload context file %s returned empty file id", filename)
	}
	return strings.TrimSpace(result.ID), nil
}

func injectCurrentInputFileContinuation(transcript string) string {
	trimmed := strings.TrimSpace(transcript)
	const summaryMarker = "Prior conversation history and tool progress."
	if idx := strings.Index(trimmed, summaryMarker); idx >= 0 {
		after := idx + len(summaryMarker)
		return trimmed[:after] + "\n" + currentInputContinuationInstruction + trimmed[after:]
	}
	return trimmed + "\n\n" + currentInputContinuationInstruction
}

func currentInputFilePrompt(hasToolsFile bool) string {
	prompt := "继续会话。HISTORY.txt contains earlier conversation context in chronological order. Follow the current system/developer instructions and continue the active user task and tool results below. If no user task follows here, the current user input is in HISTORY.txt. Match tool results by tool_call_id; a call without a matching result is not known to have completed. Do not repeat completed operations or mention the context filenames in your answer."
	if hasToolsFile {
		prompt += " TOOLS.txt is the authoritative catalog and JSON Schemas for client-provided tools in this request. Emit client tool calls only for those tools, using their declared argument types and the live tool-choice and output-format rules. Upstream native capabilities, when enabled, are separate and do not use the client tool-call format."
	}
	return prompt
}

func prependUniqueRefFileIDs(existing []string, fileIDs ...string) []string {
	out := make([]string, 0, len(existing)+len(fileIDs))
	seen := map[string]struct{}{}
	for _, fileID := range fileIDs {
		trimmed := strings.TrimSpace(fileID)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	return out
}

func replaceGeneratedCurrentInputRefs(existing []string, oldHistoryID, oldToolsID, newHistoryID, newToolsID string) []string {
	filtered := make([]string, 0, len(existing))
	old := map[string]struct{}{}
	for _, id := range []string{oldHistoryID, oldToolsID} {
		trimmed := strings.ToLower(strings.TrimSpace(id))
		if trimmed != "" {
			old[trimmed] = struct{}{}
		}
	}
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := old[strings.ToLower(trimmed)]; ok {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return prependUniqueRefFileIDs(filtered, newHistoryID, newToolsID)
}
