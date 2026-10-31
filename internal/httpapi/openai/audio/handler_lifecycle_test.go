package audio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

func TestSpeechRejectsExcessSegmentsBeforeAcquiringAccount(t *testing.T) {
	resolver := &countingAuth{accounts: 1}
	h := &Handler{Auth: resolver, DS: stubSuccessDS{}, Store: stubSingleAccountStore{stubSegmentStore{stubEnabledStore{}, 30}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"`+strings.Repeat("字", 1000)+`"}`))
	response := httptest.NewRecorder()
	h.Speech(response, req)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "too many segments") {
		t.Fatalf("expected segment validation error, got %d %s", response.Code, response.Body.String())
	}
	if got := resolver.determineCount(); got != 0 {
		t.Fatalf("invalid input consumed %d account acquisitions", got)
	}
}

type singleWorkerStore struct{ stubSegmentStore }

func (singleWorkerStore) TTSAccountConcurrencyEnabled() bool { return false }

type deletionObservation struct {
	session, token, account, installation, proxy string
}

type switchingReciteDS struct {
	stubSuccessDS
	sessions int
	deleted  chan deletionObservation
}

func (s *switchingReciteDS) CreateSession(_ context.Context, a *auth.RequestAuth, _ int) (string, error) {
	s.sessions++
	name := fmt.Sprintf("account-%d", s.sessions)
	// Simulate a completion retry switching this worker's mutable auth before
	// the next segment. Cleanup must retain the prior session's owner.
	a.AccountID, a.DeepSeekToken = name, name+"-token"
	a.Account = config.Account{Email: name, InstallationID: name + "-device", ProxyID: name + "-proxy"}
	return fmt.Sprintf("session-%d", s.sessions), nil
}

func (s *switchingReciteDS) DeleteSessionForToken(ctx context.Context, token, session string) (*dsclient.DeleteSessionResult, error) {
	a, _ := auth.FromContext(ctx)
	s.deleted <- deletionObservation{session, token, a.AccountID, a.Account.InstallationID, a.Account.ProxyID}
	return nil, nil
}

func TestSpeechCleanupKeepsEachSessionAccountIdentity(t *testing.T) {
	ds := &switchingReciteDS{deleted: make(chan deletionObservation, 2)}
	h := &Handler{Auth: &countingAuth{accounts: 1}, DS: ds, Store: singleWorkerStore{stubSegmentStore{stubEnabledStore{}, 30}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"`+strings.Repeat("字", 60)+`"}`))
	response := httptest.NewRecorder()
	h.Speech(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("speech failed: %d %s", response.Code, response.Body.String())
	}
	for range 2 {
		select {
		case observed := <-ds.deleted:
			name := strings.Replace(observed.session, "session-", "account-", 1)
			want := deletionObservation{observed.session, name + "-token", name, name + "-device", name + "-proxy"}
			if observed != want {
				t.Errorf("cleanup identity = %+v, want %+v", observed, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("session cleanup did not complete")
		}
	}
}

type cleanupContextDS struct {
	stubSuccessDS
	ctx context.Context
}

func (s *cleanupContextDS) DeleteSessionForToken(ctx context.Context, _, _ string) (*dsclient.DeleteSessionResult, error) {
	s.ctx = ctx
	return nil, nil
}

func TestDeleteSessionPreservesCleanupDeadlineAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ds := &cleanupContextDS{}
		h := &Handler{DS: ds}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		deadline, _ := ctx.Deadline()
		if canceled {
			cancel()
		}
		h.deleteSession(ctx, &auth.RequestAuth{DeepSeekToken: "token"}, "session")
		cancel()
		if got, ok := ds.ctx.Deadline(); !ok || got.After(deadline) {
			t.Errorf("cleanup extended its parent deadline: %v", got)
		}
		if canceled && ds.ctx.Err() != context.Canceled {
			t.Errorf("cleanup discarded cancellation: %v", ds.ctx.Err())
		}
	}
}

// This resolver satisfies the public auth interface, but cannot promise that
// acquiring another account will return immediately.
type noTryAuth struct{ calls int }

func (a *noTryAuth) Determine(*http.Request) (*auth.RequestAuth, error) {
	a.calls++
	if a.calls > 1 {
		return nil, errors.New("an optional acquisition would block")
	}
	return &auth.RequestAuth{AccountID: "account", DeepSeekToken: "token"}, nil
}

func (*noTryAuth) DetermineCaller(*http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{}, nil
}

func (*noTryAuth) CredentialFlagForRequest(*http.Request, string) bool { return false }
func (*noTryAuth) Release(*auth.RequestAuth)                           {}

func TestSpeechOptionalWorkersRequireNonblockingResolver(t *testing.T) {
	resolver := &noTryAuth{}
	h := &Handler{Auth: resolver, DS: stubSuccessDS{}, Store: stubSegmentStore{stubEnabledStore{}, 30}}
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"`+strings.Repeat("字", 90)+`"}`))
	response := httptest.NewRecorder()
	h.Speech(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("single-worker fallback failed: %d %s", response.Code, response.Body.String())
	}
	if resolver.calls != 1 {
		t.Fatalf("made %d potentially blocking acquisitions before starting a worker", resolver.calls)
	}
}
