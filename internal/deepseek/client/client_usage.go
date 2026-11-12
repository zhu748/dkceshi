package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/tokenusage"
)

const messageUsageTimeout = 3 * time.Second

type usageLookup struct {
	client    *Client
	auth      *auth.RequestAuth
	sessionID string
	mu        sync.Mutex
	messageID int
	usage     tokenusage.Observation
	err       error
}

// One lookup per completed message is shared by Vercel preview/release. Each
// response/generation owns its resolver; account/session changes create a new one.
func (c *Client) NewUsageResolver(a *auth.RequestAuth, sessionID string) tokenusage.Resolver {
	return &usageLookup{client: c, auth: a, sessionID: sessionID}
}

func (l *usageLookup) ResolveUpstreamUsage(ctx context.Context, id int) (tokenusage.Observation, error) {
	if id <= 0 || l.sessionID == "" || l.auth == nil {
		return tokenusage.Observation{}, errors.New("missing upstream message identity")
	}
	if err := ctx.Err(); err != nil {
		return tokenusage.Observation{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.messageID == id {
		return l.usage.Clone(), l.err
	}
	lookupCtx, cancel := context.WithTimeout(ctx, messageUsageTimeout)
	defer cancel()
	usage, err := l.client.fetchMessageUsage(lookupCtx, l.auth, l.sessionID, id)
	l.messageID, l.usage, l.err = id, usage, err
	return usage.Clone(), err
}

func (c *Client) fetchMessageUsage(ctx context.Context, a *auth.RequestAuth, sessionID string, id int) (tokenusage.Observation, error) {
	ctx = auth.WithAuth(ctx, a)
	clients := c.requestClientsForAuth(ctx, a)
	endpoint := "https://chat.deepseek.com/api/v0/chat/history_messages?" + url.Values{"chat_session_id": {sessionID}, "scenario": {"stream_close"}}.Encode()
	get := func() (map[string]any, int, error) {
		return c.getJSONWithStatus(ctx, clients.regular, endpoint, c.authHeadersForAuth(a))
	}
	payload, status, err := get()
	code, bizCode, msg, bizMsg := extractResponseStatus(payload)
	if isTokenInvalid(status, code, bizCode, msg, bizMsg) && a.UseConfigToken && c.Auth != nil && c.Auth.RefreshTokenOnce(ctx, a) {
		payload, status, err = get()
	}
	if err != nil {
		return tokenusage.Observation{}, err
	}
	code, bizCode, _, _ = extractResponseStatus(payload)
	if status != http.StatusOK || code != 0 || bizCode != 0 {
		return tokenusage.Observation{}, errors.New("upstream message usage request failed")
	}
	return tokenusage.FromHistory(payload, sessionID, id)
}

type usageResponseBody struct {
	io.ReadCloser
	tokenusage.Resolver
}
