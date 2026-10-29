package client

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"
)

// Only small metadata is retained. File bytes, signed URLs and credentials
// never enter this cache. Every non-concurrent hit is checked upstream.
type uploadReuseCache struct {
	mu         sync.Mutex
	entries    map[string]uploadReuseEntry
	flights    map[string]*uploadReuseFlight
	ttl        time.Duration
	limit      int
	generation uint64
	now        func() time.Time
}

type uploadReuseEntry struct {
	file      *UploadFileResult
	owner     string
	expiresAt time.Time
}

type uploadReuseFlight struct {
	done  chan struct{}
	file  *UploadFileResult
	err   error
	share bool
}

func newUploadReuseCacheFromEnv() *uploadReuseCache {
	ttl := uploadCacheEnvInt("DS2API_UPLOAD_CACHE_TTL_SECONDS", 900, 86400)
	limit := uploadCacheEnvInt("DS2API_UPLOAD_CACHE_MAX_ENTRIES", 256, 4096)
	if ttl == 0 || limit == 0 {
		return nil
	}
	return newUploadReuseCache(time.Duration(ttl)*time.Second, limit)
}

func uploadCacheEnvInt(name string, fallback, maximum int) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil || n < 0 {
		return fallback
	}
	return min(n, maximum)
}

func newUploadReuseCache(ttl time.Duration, limit int) *uploadReuseCache {
	return &uploadReuseCache{
		entries: make(map[string]uploadReuseEntry), flights: make(map[string]*uploadReuseFlight),
		ttl: ttl, limit: limit, now: time.Now,
	}
}

// do coalesces both uploads and metadata validation. A canceled leader cannot
// poison a live waiter, and an account-changing retry is never shared.
func (c *uploadReuseCache) do(ctx context.Context, key, owner string, load func(*UploadFileResult) (*UploadFileResult, bool, error)) (*UploadFileResult, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if flight := c.flights[key]; flight != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-flight.done:
			}
			if errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded) || (flight.err == nil && !flight.share) {
				continue
			}
			return uploadMetadataCopy(flight.file), flight.err
		}
		now := c.now()
		entry, found := c.entries[key]
		if found && !now.Before(entry.expiresAt) {
			delete(c.entries, key)
			entry = uploadReuseEntry{}
		}
		flight := &uploadReuseFlight{done: make(chan struct{})}
		c.flights[key] = flight
		generation := c.generation
		c.mu.Unlock()

		result, share, err := load(uploadMetadataCopy(entry.file))
		c.mu.Lock()
		share = share && generation == c.generation
		if err == nil && share && result != nil && result.ID != "" && !isFailedUploadFileStatus(result.Status) {
			expiresAt := entry.expiresAt
			if entry.file == nil || entry.file.ID != result.ID {
				expiresAt = c.now().Add(c.ttl)
			}
			c.putLocked(key, uploadReuseEntry{file: uploadMetadataCopy(result), owner: owner, expiresAt: expiresAt})
		}
		flight.file, flight.share, flight.err = uploadMetadataCopy(result), share, err
		delete(c.flights, key)
		close(flight.done)
		c.mu.Unlock()
		return result, err
	}
}

func (c *uploadReuseCache) putLocked(key string, entry uploadReuseEntry) {
	now := c.now()
	for k, value := range c.entries {
		if !now.Before(value.expiresAt) {
			delete(c.entries, k)
		}
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.limit {
		oldestKey := ""
		var oldest time.Time
		for k, value := range c.entries {
			if oldestKey == "" || value.expiresAt.Before(oldest) {
				oldestKey, oldest = k, value.expiresAt
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[key] = entry
}

func (c *uploadReuseCache) forget(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *uploadReuseCache) invalidateOwner(owner string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++ // Prevent an in-flight upload/validation from repopulating.
	for key, entry := range c.entries {
		if entry.owner == owner {
			delete(c.entries, key)
		}
	}
}

func uploadMetadataCopy(file *UploadFileResult) *UploadFileResult {
	if file == nil {
		return nil
	}
	copy := *file
	copy.Raw, copy.RawHeaders = nil, nil
	return &copy
}
