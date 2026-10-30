package files

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	defaultContentStoreTotalBytes = 256 << 20
	defaultContentStoreEntries    = 256
)

// ErrFileNotFound is returned when a file ID is not present in the store.
var ErrFileNotFound = errors.New("file content not found")

// ErrFileTooLarge is returned when a file exceeds the per-file byte limit.
var ErrFileTooLarge = errors.New("file content exceeds size limit")

// ContentStore keeps uploaded file bytes in memory so that request handlers can
// later read them back without depending on upstream file download APIs.
type ContentStore interface {
	Store(id string, filename, mimeType string, data []byte) error
	Read(id string) (filename, mimeType string, data []byte, err error)
}

// MemoryContentStore owns its bytes and bounds both total bytes and entry count.
// Expired entries are reclaimed on writes; reads do not extend the TTL.
type MemoryContentStore struct {
	mu            sync.RWMutex
	entries       map[string]*memoryContentEntry
	maxBytes      int
	maxTotalBytes int
	maxEntries    int
	totalBytes    int
	ttl           time.Duration
}

type memoryContentEntry struct {
	filename  string
	mimeType  string
	data      []byte
	expiresAt time.Time
}

// NewMemoryContentStore creates a store that caps each stored file at maxBytes
// and expires entries ttl after they are written. The whole cache is limited to
// 256 MiB / 256 entries, evicting the entry nearest expiry when full.
func NewMemoryContentStore(maxBytes int, ttl time.Duration) *MemoryContentStore {
	return &MemoryContentStore{
		entries:       make(map[string]*memoryContentEntry),
		maxBytes:      maxBytes,
		maxTotalBytes: defaultContentStoreTotalBytes,
		maxEntries:    defaultContentStoreEntries,
		ttl:           ttl,
	}
}

// Store saves file bytes keyed by id. Empty ids are rejected. Files larger than
// maxBytes are rejected with ErrFileTooLarge.
func (s *MemoryContentStore) Store(id string, filename, mimeType string, data []byte) error {
	if id = strings.TrimSpace(id); id == "" {
		return errors.New("file id is required")
	}
	if (s.maxBytes > 0 && len(data) > s.maxBytes) || len(data) > s.maxTotalBytes {
		return ErrFileTooLarge
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(time.Now())
	s.deleteLocked(id)
	for len(s.entries) >= s.maxEntries || s.totalBytes+len(data) > s.maxTotalBytes {
		oldest := ""
		var expiry time.Time
		for key, entry := range s.entries {
			if oldest == "" || entry.expiresAt.Before(expiry) {
				oldest, expiry = key, entry.expiresAt
			}
		}
		s.deleteLocked(oldest)
	}
	s.entries[id] = &memoryContentEntry{
		filename:  filename,
		mimeType:  mimeType,
		data:      bytes.Clone(data),
		expiresAt: time.Now().Add(s.ttl),
	}
	s.totalBytes += len(data)
	return nil
}

// Read returns the stored file content and metadata. Expired entries are
// treated as missing.
func (s *MemoryContentStore) Read(id string) (filename, mimeType string, data []byte, err error) {
	if id = strings.TrimSpace(id); id == "" {
		return "", "", nil, ErrFileNotFound
	}
	s.mu.RLock()
	entry, ok := s.entries[id]
	s.mu.RUnlock()
	if !ok || entry == nil {
		return "", "", nil, ErrFileNotFound
	}
	if !time.Now().Before(entry.expiresAt) {
		s.mu.Lock()
		// Re-check under write lock in case another goroutine refreshed it.
		if e, stillThere := s.entries[id]; stillThere && e == entry {
			s.deleteLocked(id)
		}
		s.mu.Unlock()
		return "", "", nil, ErrFileNotFound
	}
	return entry.filename, entry.mimeType, bytes.Clone(entry.data), nil
}

// Delete removes a file from the store. It is safe to call for a missing id.
func (s *MemoryContentStore) Delete(id string) {
	if id = strings.TrimSpace(id); id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(id)
}

// CleanupExpired removes all entries past their TTL.
func (s *MemoryContentStore) CleanupExpired() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(now)
}

func (s *MemoryContentStore) deleteLocked(id string) {
	if entry, ok := s.entries[id]; ok {
		s.totalBytes -= len(entry.data)
		delete(s.entries, id)
	}
}

func (s *MemoryContentStore) cleanupExpiredLocked(now time.Time) {
	for id, entry := range s.entries {
		if !now.Before(entry.expiresAt) {
			s.deleteLocked(id)
		}
	}
}
