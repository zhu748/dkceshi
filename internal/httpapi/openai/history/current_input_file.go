package history

import (
        "context"
        "errors"
        "fmt"
        "strings"
        "time"

        "ds2api/internal/auth"
        "ds2api/internal/config"
        dsclient "ds2api/internal/deepseek/client"
        "ds2api/internal/httpapi/openai/shared"
        "ds2api/internal/promptcompat"
)

const (
        // currentInputFilename / currentToolsFilename 不再使用固定字面值，
        // 真实 Android App 抓包显示用户上传的文件名都是简短风格（如 111.txt / 222.txt），
        // 而早期实现写死 chat_context.txt / tool_schema.txt 是极强指纹。
        // 现在改由 makeRandomShortFilename 在每次上传时动态生成。
        currentInputContentType = "application/octet-stream"
        currentInputPurpose     = "assistants"
)

// makeRandomShortFilename 生成形如 "111.txt" / "4527.txt" 的短数字文件名，
// 对齐真实 App 抓包里观察到的用户文件命名风格（111.txt / 222.txt 等）。
// 不再暴露 chat_context / tool_schema 等结构化命名，避免被文件名维度风控。
func makeRandomShortFilename() string {
        const minN = 100
        const maxN = 9999
        // 使用时间戳低位作为种子抖动，避免同一进程内连续上传出现同名。
        seed := time.Now().UnixNano()
        rng := seed%int64(maxN-minN+1) + int64(minN)
        return fmt.Sprintf("%d.txt", rng)
}

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
        // 降级机制：该账号近期上传失败次数过多时，本次请求自动降级到模式 A
        // （即不进入文件上传分支，由调用方按原 stdReq.FinalPrompt 走纯文本路径）。
        // 这能避免单账号连续上传失败触发更严的风控升级（如直接封号）。
        if globalDegradeTracker.IsDegraded(a.AccountID) {
                return stdReq, nil
        }
        threshold := s.Store.CurrentInputFileMinChars()

        index, text := latestUserInputForFile(stdReq.Messages)
        if index < 0 {
                return stdReq, nil
        }
        if len([]rune(text)) < threshold {
                return stdReq, nil
        }
        fileText := promptcompat.BuildOpenAICurrentInputContextTranscript(stdReq.Messages)
        if strings.TrimSpace(fileText) == "" {
                return stdReq, errors.New("current user input file produced empty transcript")
        }
        toolsText, _ := promptcompat.BuildOpenAIToolsContextTranscript(stdReq.ToolsRaw, stdReq.ToolChoice)
        modelType := "default"
        if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
                modelType = resolvedType
        }

        // 节流抖动：在上传前引入 50-200ms 随机延迟，打破 "上传即 completion" 的
        // 固定时序模式。真实 App 用户从选文件到点发送之间通常有可观察的延迟。
        jitterSleep()

        // 文件复用：同账号 + 同内容（hash）在 TTL（10min）内复用 fileID，
        // 避免短时间内重复上传相同上下文文件（同一对话回合多次 retry 场景特别常见）。
        fileID := fileUploadCache.Lookup(a.AccountID, fileText)
        if fileID == "" {
                result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
                        Filename:    makeRandomShortFilename(),
                        ContentType: currentInputContentType,
                        Purpose:     currentInputPurpose,
                        ModelType:   modelType,
                        Data:        []byte(fileText),
                }, 3)
                if err != nil {
                        globalDegradeTracker.RecordFailure(a.AccountID)
                        return stdReq, fmt.Errorf("upload current user input file: %w", err)
                }
                fileID = strings.TrimSpace(result.ID)
                if fileID == "" {
                        globalDegradeTracker.RecordFailure(a.AccountID)
                        return stdReq, errors.New("upload current user input file returned empty file id")
                }
                fileUploadCache.Store(a.AccountID, fileText, fileID)
                globalDegradeTracker.RecordSuccess(a.AccountID)
        }

        toolFileID := ""
        if strings.TrimSpace(toolsText) != "" {
                toolFileID = fileUploadCache.Lookup(a.AccountID, toolsText)
                if toolFileID == "" {
                        result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
                                Filename:    makeRandomShortFilename(),
                                ContentType: currentInputContentType,
                                Purpose:     currentInputPurpose,
                                ModelType:   modelType,
                                Data:        []byte(toolsText),
                        }, 3)
                        if err != nil {
                                globalDegradeTracker.RecordFailure(a.AccountID)
                                return stdReq, fmt.Errorf("upload current tools file: %w", err)
                        }
                        toolFileID = strings.TrimSpace(result.ID)
                        if toolFileID == "" {
                                globalDegradeTracker.RecordFailure(a.AccountID)
                                return stdReq, errors.New("upload current tools file returned empty file id")
                        }
                        fileUploadCache.Store(a.AccountID, toolsText, toolFileID)
                        globalDegradeTracker.RecordSuccess(a.AccountID)
                }
        }

        messages := []any{
                map[string]any{
                        "role":    "user",
                        "content": currentInputFilePrompt(toolFileID != ""),
                },
        }

        stdReq.Messages = messages
        stdReq.HistoryText = fileText
        stdReq.CurrentInputFileApplied = true
        stdReq.CurrentInputFileID = fileID
        stdReq.CurrentToolsFileID = toolFileID
        stdReq.RefFileIDs = prependUniqueRefFileIDs(stdReq.RefFileIDs, fileID, toolFileID)
        stdReq.FinalPrompt, stdReq.ToolNames = promptcompat.BuildOpenAIPromptWithToolInstructionsOnly(messages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking)
        // Token accounting must reflect the actual downstream context:
        // uploaded context files + the continuation live prompt.
        tokenParts := []string{fileText}
        if strings.TrimSpace(toolsText) != "" {
                tokenParts = append(tokenParts, toolsText)
        }
        tokenParts = append(tokenParts, stdReq.FinalPrompt)
        stdReq.PromptTokenText = strings.Join(tokenParts, "\n")
        return stdReq, nil
}

func (s Service) ReuploadAppliedCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
        if !stdReq.CurrentInputFileApplied || s.DS == nil || a == nil {
                return stdReq, nil
        }
        fileText := strings.TrimSpace(stdReq.HistoryText)
        if fileText == "" {
                return stdReq, nil
        }
        modelType := "default"
        if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
                modelType = resolvedType
        }

        // Reupload 路径同样走缓存：账号切换后新账号下若已上传过同内容文件，
        // 直接复用 fileID，避免 retry 风暴下产生大量重复上传。
        fileID := fileUploadCache.Lookup(a.AccountID, stdReq.HistoryText)
        if fileID == "" {
                result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
                        Filename:    makeRandomShortFilename(),
                        ContentType: currentInputContentType,
                        Purpose:     currentInputPurpose,
                        ModelType:   modelType,
                        Data:        []byte(stdReq.HistoryText),
                }, 3)
                if err != nil {
                        globalDegradeTracker.RecordFailure(a.AccountID)
                        return stdReq, fmt.Errorf("upload current user input file: %w", err)
                }
                fileID = strings.TrimSpace(result.ID)
                if fileID == "" {
                        globalDegradeTracker.RecordFailure(a.AccountID)
                        return stdReq, errors.New("upload current user input file returned empty file id")
                }
                fileUploadCache.Store(a.AccountID, stdReq.HistoryText, fileID)
                globalDegradeTracker.RecordSuccess(a.AccountID)
        }

        toolsText, _ := promptcompat.BuildOpenAIToolsContextTranscript(stdReq.ToolsRaw, stdReq.ToolChoice)
        toolFileID := ""
        if strings.TrimSpace(toolsText) != "" {
                toolFileID = fileUploadCache.Lookup(a.AccountID, toolsText)
                if toolFileID == "" {
                        result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
                                Filename:    makeRandomShortFilename(),
                                ContentType: currentInputContentType,
                                Purpose:     currentInputPurpose,
                                ModelType:   modelType,
                                Data:        []byte(toolsText),
                        }, 3)
                        if err != nil {
                                globalDegradeTracker.RecordFailure(a.AccountID)
                                return stdReq, fmt.Errorf("upload current tools file: %w", err)
                        }
                        toolFileID = strings.TrimSpace(result.ID)
                        if toolFileID == "" {
                                globalDegradeTracker.RecordFailure(a.AccountID)
                                return stdReq, errors.New("upload current tools file returned empty file id")
                        }
                        fileUploadCache.Store(a.AccountID, toolsText, toolFileID)
                        globalDegradeTracker.RecordSuccess(a.AccountID)
                }
        }

        stdReq.RefFileIDs = replaceGeneratedCurrentInputRefs(stdReq.RefFileIDs, stdReq.CurrentInputFileID, stdReq.CurrentToolsFileID, fileID, toolFileID)
        stdReq.CurrentInputFileID = fileID
        stdReq.CurrentToolsFileID = toolFileID
        return stdReq, nil
}

func latestUserInputForFile(messages []any) (int, string) {
        for i := len(messages) - 1; i >= 0; i-- {
                msg, ok := messages[i].(map[string]any)
                if !ok {
                        continue
                }
                role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
                if role != "user" {
                        continue
                }
                text := promptcompat.NormalizeOpenAIContentForPrompt(msg["content"])
                if strings.TrimSpace(text) == "" {
                        return -1, ""
                }
                return i, text
        }
        return -1, ""
}

func currentInputFilePrompt(hasToolsFile bool) string {
        // 早期实现直接字面提到 "chat_context.txt" / "tool_schema.txt" 这种结构化文件名，
        // 真实 App 用户从不会写出这种 prompt。现在改为更自然的引导句，
        // 不暴露内部使用的文件命名约定。
        prompt := "The attached file contains the prior conversation. Read it and answer the most recent user request directly."
        if hasToolsFile {
                prompt += " The other attached file lists available function definitions and parameter contracts; only use those tools and follow the function-call contract described below."
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
