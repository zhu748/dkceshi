// history-replay inspects exported response histories without invoking tools or
// contacting an upstream service. Argument values are represented by digests.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"ds2api/internal/assistantturn"
	"ds2api/internal/chathistory"
	"ds2api/internal/jsonvalue"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

type callAudit struct {
	Name            string   `json:"name"`
	ParameterNames  []string `json:"parameter_names"`
	ArgumentsSHA256 string   `json:"arguments_sha256"`
}

type entryAudit struct {
	ID                    string      `json:"id"`
	Surface               string      `json:"surface"`
	DirectCallCount       int         `json:"direct_call_count"`
	Calls                 []callAudit `json:"calls"`
	DSMLRecovered         bool        `json:"dsml_recovered"`
	HadTranscriptBoundary bool        `json:"had_transcript_boundary"`
	HasToolSchemas        bool        `json:"has_tool_schemas"`
	Messages              int         `json:"messages"`
	ToolOnlyTurns         int         `json:"archived_tool_only_turns"`
	PromptTokens          int         `json:"recorded_prompt_tokens"`
	VisibleCharacters     int         `json:"replayed_visible_characters"`
	ErrorCode             string      `json:"replay_error_code,omitempty"`
}

type auditReport struct {
	Records           int          `json:"records"`
	MainRequests      int          `json:"main_requests"`
	RepairRequests    int          `json:"repair_requests"`
	RepairElapsedMs   int64        `json:"recorded_repair_elapsed_ms"`
	RecoveredRequests int          `json:"main_requests_recovered_locally"`
	MainToolCalls     int          `json:"replayed_main_tool_calls"`
	Entries           []entryAudit `json:"entries"`
}

func main() {
	archive := flag.String("archive", "", "path to exported chat_history.zip")
	flag.Parse()
	if *archive == "" {
		flag.Usage()
		os.Exit(2)
	}
	entries, err := loadArchive(*archive)
	if err != nil {
		fail(err)
	}
	report, err := audit(entries)
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fail(err)
	}
}

func fail(err error) {
	if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
		panic(writeErr)
	}
	os.Exit(1)
}

func loadArchive(path string) (entries []chathistory.Entry, err error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, archive.Close()) }()
	const maxRecordBytes = 32 << 20
	const maxArchiveBytes = 256 << 20
	total := 0
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "chat_history.json.d/") || !strings.HasSuffix(file.Name, ".json") {
			continue
		}
		data, readErr := readRecord(file, maxRecordBytes)
		if readErr != nil {
			return nil, fmt.Errorf("%s: %w", file.Name, readErr)
		}
		total += len(data)
		if total > maxArchiveBytes {
			return nil, fmt.Errorf("archive exceeds %d decompressed bytes", maxArchiveBytes)
		}
		var envelope struct{ Item chathistory.Entry }
		if err := jsonvalue.Unmarshal(data, &envelope); err != nil {
			return nil, fmt.Errorf("%s: %w", file.Name, err)
		}
		entries = append(entries, envelope.Item)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no response-history detail records found")
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].CreatedAt == entries[j].CreatedAt {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].CreatedAt < entries[j].CreatedAt
	})
	return entries, nil
}

func readRecord(file *zip.File, limit int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, limit+1))
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("record exceeds %d bytes", limit)
	}
	return data, nil
}

func audit(entries []chathistory.Entry) (auditReport, error) {
	report := auditReport{Records: len(entries), Entries: []entryAudit{}}
	for _, entry := range entries {
		var tools []any
		if len(entry.Tools) > 0 {
			if err := jsonvalue.Unmarshal(entry.Tools, &tools); err != nil {
				return report, fmt.Errorf("%s tools: %w", entry.ID, err)
			}
		}
		policy := promptcompat.DefaultToolChoicePolicy()
		if len(entry.ToolChoice) > 0 {
			if err := jsonvalue.Unmarshal(entry.ToolChoice, &policy); err != nil {
				return report, fmt.Errorf("%s tool policy: %w", entry.ID, err)
			}
		}
		raw, _ := sse.TrimToolRoleMarker(toolcall.NormalizeMarkerText(entry.Content, entry.ToolMarker))
		direct := toolcall.ParseAssistantToolCallsDetailed(raw, entry.ReasoningContent, nil)
		_, recovered := toolcall.RecoverDSMLToolCalls(raw)
		turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: entry.Content, Thinking: entry.ReasoningContent}, assistantturn.BuildOptions{
			Model: entry.Model, ToolsRaw: tools, ToolChoice: policy, ToolMarker: entry.ToolMarker,
		})
		var guard sse.ToolRoleMarkerGuard
		guard.Feed(entry.Content)
		item := entryAudit{ID: entry.ID, Surface: entry.Surface, DirectCallCount: len(direct.Calls), Calls: []callAudit{}, DSMLRecovered: recovered, HadTranscriptBoundary: guard.Reason() == sse.CutTranscriptBoundary, HasToolSchemas: len(tools) > 0, Messages: len(entry.Messages), VisibleCharacters: utf8.RuneCountInString(turn.Text)}
		if turn.Error != nil {
			item.ErrorCode = turn.Error.Code
		}
		if value, ok := entry.Usage["prompt_tokens"].(json.Number); ok {
			count, err := value.Int64()
			if err == nil {
				item.PromptTokens = int(count)
			}
		}
		for _, msg := range entry.Messages {
			if msg.Role == "assistant" && msg.Content == "" && len(msg.ToolCalls) > 0 {
				var calls []json.RawMessage
				if err := json.Unmarshal(msg.ToolCalls, &calls); err == nil && len(calls) > 0 {
					item.ToolOnlyTurns++
				}
			}
		}
		for _, call := range turn.ToolCalls {
			keys := make([]string, 0, len(call.Input))
			for key := range call.Input {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			encoded, err := json.Marshal(call.Input)
			if err != nil {
				return report, err
			}
			item.Calls = append(item.Calls, callAudit{Name: call.Name, ParameterNames: keys, ArgumentsSHA256: fmt.Sprintf("%x", sha256.Sum256(encoded))})
		}
		if entry.Surface == "toolcall.repair" {
			report.RepairRequests++
			report.RepairElapsedMs += entry.ElapsedMs
		} else {
			report.MainRequests++
			report.MainToolCalls += len(turn.ToolCalls)
			if recovered {
				report.RecoveredRequests++
			}
		}
		report.Entries = append(report.Entries, item)
	}
	return report, nil
}
