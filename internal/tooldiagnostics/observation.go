package tooldiagnostics

import (
	"context"
	"time"
)

// Observation is produced by the shared finalizer, after the actual policy and
// schema checks. Intermediate turn builds must not submit observations.
type Observation struct {
	Relevant   bool
	Model      string
	Source     string
	Raw        string
	Residual   string
	LocalSteps []string
	Calls      []Call
	ErrorCode  string
	Error      string
}

// A transport failure must not replace a validated attempt's calls or repair
// evidence with the Node release payload, which contains only final text.
func TransportFailure(ctx context.Context, o Observation) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	observed := r.current().CompletedAt != 0
	r.mu.Unlock()
	if !observed {
		Observe(ctx, o)
	}
	Failure(ctx, o.ErrorCode, o.Error)
}

func Observe(ctx context.Context, o Observation) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || r.record.CompletedAt != 0 || !o.Relevant && !r.active {
		return
	}
	if !o.Relevant && o.ErrorCode != "" && r.current().Source != "" {
		r.failLocked(o.ErrorCode, o.Error)
		return
	}
	if !o.Relevant && o.ErrorCode == "" && r.discardCandidateLocked() {
		return
	}
	r.active = true
	r.record.Model = o.Model
	a := r.current()
	a.Raw, a.Residual, a.Source = Capture(o.Raw), Capture(o.Residual), o.Source
	a.Calls = o.Calls
	if a.Calls == nil {
		a.Calls = []Call{}
	}
	a.Method = "direct"
	if len(o.LocalSteps) > 0 {
		a.Method = "local_recovery"
	}
	for _, step := range a.Steps {
		if step.Stage == "local_recovery" && step.Result == "success" {
			a.Method = "local_recovery"
		}
		if step.Stage == "llm_repair" && step.Result == "success" {
			a.Method = "llm_repair"
		}
	}
	// Replace finalizer observations while retaining repair/retry events.
	kept := a.Steps[:0]
	for _, step := range a.Steps {
		if step.Stage != "parse" && step.Stage != "validation" {
			kept = append(kept, step)
		}
	}
	a.Steps = kept
	for _, reason := range o.LocalSteps {
		a.Steps = append(a.Steps, Step{Stage: "parse", Result: "recovered", Reason: reason, At: time.Now().UnixMilli()})
	}
	parseResult := "success"
	if len(a.Calls) == 0 {
		parseResult = "no_calls"
	}
	if o.Residual != "" {
		parseResult = "residual"
	}
	a.Steps = append(a.Steps, Step{Stage: "parse", Result: parseResult, At: time.Now().UnixMilli()})
	a.ErrorCode, a.Error = o.ErrorCode, o.Error
	a.Result = "success"
	a.CompletedAt = time.Now().UnixMilli()
	if o.ErrorCode != "" {
		a.Result = "failed"
		r.record.Pinned = true
		r.record.ErrorCode, r.record.Error = o.ErrorCode, o.Error
	}
	a.Steps = append(a.Steps, Step{Stage: "validation", Result: a.Result, Reason: o.ErrorCode, Error: o.Error, At: time.Now().UnixMilli()})
	for _, call := range a.Calls {
		found := false
		for _, name := range r.record.ToolNames {
			if name == call.Name {
				found = true
			}
		}
		if !found {
			r.record.ToolNames = append(r.record.ToolNames, call.Name)
		}
	}
	r.persistLocked()
}
