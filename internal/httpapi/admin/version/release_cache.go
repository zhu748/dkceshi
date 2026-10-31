package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const releaseCheckTimeout = 4 * time.Second

type releaseStatus struct {
	release   latestReleasePayload
	checkedAt time.Time
	pending   bool
	err       string
}

type releaseCache struct {
	mu        sync.Mutex
	status    releaseStatus
	nextCheck time.Time
}

// The local version never waits for the network. One bounded background check
// refreshes the cache; retain the last successful release during outages.
func (c *releaseCache) snapshot(check func(context.Context) (latestReleasePayload, error)) releaseStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.status.pending && !time.Now().Before(c.nextCheck) {
		c.status.pending = true
		go c.refresh(check)
	}
	return c.status
}

func (c *releaseCache) refresh(check func(context.Context) (latestReleasePayload, error)) {
	ctx, cancel := context.WithTimeout(context.Background(), releaseCheckTimeout)
	defer cancel()
	release, err := check(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.pending = false
	c.status.checkedAt = time.Now()
	if err != nil {
		c.status.err = err.Error()
		c.nextCheck = c.status.checkedAt.Add(time.Minute)
		return
	}
	c.status.release, c.status.err = release, ""
	c.nextCheck = c.status.checkedAt.Add(15 * time.Minute)
}

func fetchLatestRelease(ctx context.Context) (latestReleasePayload, error) {
	var data latestReleasePayload
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseAPI, nil)
	if err != nil {
		return data, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ds2api-version-check")
	client := &http.Client{Timeout: releaseCheckTimeout}
	r, err := client.Do(req)
	if err != nil {
		return data, err
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
	}()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return data, fmt.Errorf("github api status: %s", r.Status)
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&data); err != nil {
		return data, err
	}
	if strings.TrimSpace(data.TagName) == "" {
		return data, fmt.Errorf("missing latest tag")
	}
	return data, nil
}
