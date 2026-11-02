package completionruntime

import (
	"context"
	"errors"
	"time"

	"ds2api/internal/toolcall"
	"ds2api/internal/tooldiagnostics"
)

type RepairAPIProbe struct {
	Success   bool   `json:"success"`
	Protocol  string `json:"protocol"`
	Model     string `json:"model"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

// Exercise the exact generation adapter and format-integrity checks with a
// fixed harmless example. No account, main conversation or saved setting is used.
func TestRepairAPI(ctx context.Context, settings tooldiagnostics.RepairSettings) (RepairAPIProbe, error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(settings.TimeoutSeconds)*time.Second)
	defer cancel()
	const sample = `<|EPSE|broken><|EPSE|invoke name="repair_probe"><|EPSE|parameter name="path"><![CDATA[probe.txt]]></|EPSE|parameter></|EPSE|invoke></|EPSE|broken>`
	var invokeError error
	calls, ok := toolcall.RepairToolCallsWithLLM(ctx, sample, func(ctx context.Context, prompt string) (string, error) {
		output, err := callRepairAPI(ctx, settings, prompt)
		invokeError = err
		return output.Text, err
	})
	result := RepairAPIProbe{Protocol: tooldiagnostics.RepairProtocol(settings.Protocol), Model: settings.Model, ElapsedMS: time.Since(started).Milliseconds()}
	if invokeError != nil {
		return result, invokeError
	}
	if !ok || len(calls) != 1 || calls[0].Name != "repair_probe" || len(calls[0].Input) != 1 || calls[0].Input["path"] != "probe.txt" {
		return result, errors.New("repair API responded, but the sample failed format or argument integrity validation")
	}
	result.Success = true
	return result, nil
}
