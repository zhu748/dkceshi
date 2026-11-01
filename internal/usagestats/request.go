package usagestats

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/util"
)

// RequestRecorder keeps attribution live until completion: retries can replace
// the account in RequestAuth. Only the final effective call is recorded, once.
// Failed attempts, keep-alive and repair requests are not consumption entries.
type RequestRecorder struct {
	store *Store
	auth  *auth.RequestAuth
	model string
	at    time.Time
	once  sync.Once
}

func NewRequestRecorder(store *Store, a *auth.RequestAuth, model string, at time.Time) *RequestRecorder {
	if store == nil || a == nil {
		return nil
	}
	return &RequestRecorder{store: store, auth: a, model: model, at: at}
}

func (r *RequestRecorder) Record(usage map[string]any) {
	if r == nil {
		return
	}
	if _, _, _, total := ParseUsage(usage); total <= 0 {
		return
	}
	r.once.Do(func() {
		id, label := usageAccount(r.auth)
		r.store.RecordAccount(r.model, r.auth.CallerID, id, label, usage, r.at)
	})
}

// ModelForRequest groups model aliases by their resolved upstream model.
func ModelForRequest(resolved, response string) string {
	if model := strings.TrimSpace(resolved); model != "" {
		return model
	}
	return normalizeModel(response)
}

func usageAccount(a *auth.RequestAuth) (string, string) {
	if !a.UseConfigToken {
		return "direct:" + normalizeCallerID(a.CallerID), ""
	}
	id := strings.TrimSpace(a.AccountID)
	if id == "" {
		return "unknown", ""
	}
	sum := sha256.Sum256([]byte(id))
	masked := maskAccount(id)
	if name := strings.TrimSpace(a.Account.Name); name != "" {
		masked = strings.ReplaceAll(name, id, masked) + " · " + masked
	}
	return "account:" + hex.EncodeToString(sum[:16]), masked
}

func maskAccount(id string) string {
	if local, domain, ok := strings.Cut(id, "@"); ok {
		chars := []rune(local)
		if len(chars) > 0 {
			return string(chars[:1]) + "***@" + domain
		}
		return "***@" + domain
	}
	chars := []rune(id)
	if len(chars) > 7 {
		return string(chars[:3]) + "****" + string(chars[len(chars)-4:])
	}
	return "***"
}

// PartialUsage uses the current normalized request when a stream is cancelled.
// PromptTokenText includes uploaded context/retry prompts; reasoning is already
// part of completion_tokens. An optional current request replaces the initial
// accounting context without changing the archived original request.
func PartialUsage(initial promptcompat.StandardRequest, thinking, content string, current ...promptcompat.StandardRequest) map[string]any {
	request := initial
	if len(current) > 0 {
		request = current[0]
		if request.RefFileTokens == 0 {
			request.RefFileTokens = initial.RefFileTokens
		}
	}
	prompt := request.PromptTokenText
	if prompt == "" {
		prompt = request.FinalPrompt
	}
	model := ModelForRequest(initial.ResolvedModel, initial.ResponseModel)
	input := util.CountPromptTokens(prompt, model) + request.RefFileTokens
	reasoning := util.CountOutputTokens(thinking, model)
	output := reasoning + util.CountOutputTokens(content, model)
	return map[string]any{"prompt_tokens": input, "completion_tokens": output, "total_tokens": input + output,
		"completion_tokens_details": map[string]any{"reasoning_tokens": reasoning}}
}
