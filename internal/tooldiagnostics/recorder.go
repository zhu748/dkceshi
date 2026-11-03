package tooldiagnostics

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

type contextKey struct{}

type Recorder struct {
	mu              sync.Mutex
	store           *Store
	record          Record
	active          bool
	number          int
	flushTimer      *time.Timer
	flushVersion    uint64
	persistFailures int
}

func NewRecorder(store *Store, surface string, stream bool) *Recorder {
	if !store.Enabled() {
		return nil
	}
	now := time.Now().UnixMilli()
	return &Recorder{store: store, number: 1, record: Record{Summary: Summary{ID: "tool_" + uuid.NewString(), CreatedAt: now, UpdatedAt: now, Surface: surface, Stream: stream, Status: "running", Delivery: "pending", ToolNames: []string{}}, Attempts: []Attempt{newAttempt(1, "initial")}}}
}

func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	return context.WithValue(ctx, contextKey{}, recorder)
}
func FromContext(ctx context.Context) *Recorder {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(contextKey{}).(*Recorder)
	return r
}
func newAttempt(number int, kind string) Attempt {
	return Attempt{Number: number, Kind: kind, StartedAt: time.Now().UnixMilli(), Result: "running", Calls: []Call{}, Steps: []Step{}}
}

func Configure(ctx context.Context, model, caller, account string, stream bool, tools, policy any) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || r.record.CompletedAt != 0 {
		return
	}
	r.record.Model, r.record.CallerID, r.record.Stream = model, caller, stream
	r.current().AccountID = account
	if tools != nil {
		raw, err := json.Marshal(tools)
		if err == nil {
			r.record.Tools = Capture(string(raw))
		}
	}
	if policy != nil {
		raw, err := json.Marshal(policy)
		if err == nil {
			r.record.Policy = Capture(string(raw))
		}
	}
}

func (r *Recorder) current() *Attempt { return &r.record.Attempts[len(r.record.Attempts)-1] }

func AddStep(ctx context.Context, step Step) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || r.record.CompletedAt != 0 {
		return
	}
	first := !r.active
	r.active = true
	if step.At == 0 {
		step.At = time.Now().UnixMilli()
	}
	a := r.current()
	if len(a.Steps) < 64 {
		a.Steps = append(a.Steps, step)
	}
	if first {
		r.persistLocked()
	} else {
		r.schedulePersistLocked()
	}
}

func NextAttempt(ctx context.Context, kind, code, reason, account string) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || r.record.CompletedAt != 0 || len(r.record.Attempts) >= 32 {
		return
	}
	r.advanceLocked(kind, code, reason, account)
}

// SelectAttempt supports lease-bound Node validations. Repeated validation of
// the same attempt updates that attempt instead of duplicating the request.
func SelectAttempt(ctx context.Context, number int, reason, account string) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	if number < 1 || number > 32 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.number < number && r.record.CompletedAt == 0 {
		r.advanceLocked("regeneration", "tool_output_retry", reason, account)
	}
}

func Failure(ctx context.Context, code, message string) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || !r.active || r.record.CompletedAt != 0 {
		return
	}
	r.failLocked(code, message)
}

func (r *Recorder) failLocked(code, message string) {
	a := r.current()
	if a.Result == "failed" && a.ErrorCode == code && a.Error == message {
		return
	}
	a.Result, a.ErrorCode, a.Error = "failed", code, message
	a.CompletedAt = time.Now().UnixMilli()
	r.record.Pinned = true
	r.record.ErrorCode, r.record.Error = code, message
	r.persistLocked()
}

func RetryDecision(ctx context.Context, decision, reason string) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || !r.active || r.record.CompletedAt != 0 {
		return
	}
	r.current().RetryDecision, r.current().RetryReason = decision, reason
	r.schedulePersistLocked()
}

func (r *Recorder) Finish(status int, deliveryError error, stopped bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active || r.record.CompletedAt != 0 {
		return
	}
	a := r.current()
	now := time.Now().UnixMilli()
	r.record.CompletedAt = now
	if a.CompletedAt == 0 {
		a.CompletedAt = now
	}
	switch {
	case stopped:
		a.Result = "interrupted"
		r.record.Status, r.record.Delivery, r.record.Pinned = "interrupted", "unknown", true
		if a.RetryDecision == "" {
			a.RetryDecision = "cancelled"
		}
	case deliveryError != nil:
		a.Result, a.ErrorCode, a.Error = "failed", "response_write_failed", deliveryError.Error()
		r.record.Status, r.record.Delivery, r.record.Pinned = "failed", "write_failed", true
		r.record.ErrorCode, r.record.Error = "response_write_failed", deliveryError.Error()
	case status >= 400 || a.Result == "failed" || a.Result == "running":
		a.Result = "failed"
		if a.ErrorCode == "" {
			a.ErrorCode, a.Error = "request_incomplete", "Request ended before tool delivery completed."
		}
		r.record.ErrorCode, r.record.Error = a.ErrorCode, a.Error
		r.record.Status, r.record.Delivery, r.record.Pinned = "failed", "not_delivered", true
		if a.RetryDecision == "" {
			a.RetryDecision = "not_retried"
		}
	case r.number > 1:
		r.record.Status, r.record.Delivery = "regenerated_success", "written"
	default:
		r.record.Status, r.record.Delivery = a.Method+"_success", "written"
	}
	if r.record.Delivery == "written" {
		for i := range a.Calls {
			a.Calls[i].Delivery = "written"
		}
	}
	r.persistLocked()
}

func (r *Recorder) advanceLocked(kind, code, reason, account string) {
	a := r.current()
	if a.CompletedAt == 0 {
		a.CompletedAt = time.Now().UnixMilli()
		a.Result, a.ErrorCode, a.Error = "failed", code, reason
		r.record.Pinned = true
		r.record.ErrorCode, r.record.Error = code, reason
	}
	a.RetryDecision = "started"
	a.RetryReason = reason
	if code != "" {
		a.Steps = append(a.Steps, Step{Stage: "retry", Result: "started", Reason: code, Error: reason, At: time.Now().UnixMilli()})
	}
	r.number++
	next := newAttempt(r.number, kind)
	next.AccountID = account
	r.record.Attempts = append(r.record.Attempts, next)
	r.schedulePersistLocked()
}
