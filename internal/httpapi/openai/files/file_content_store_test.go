package files

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemoryContentStoreOwnsBytes(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Hour)
	input := []byte("original")
	if err := store.Store("file", "a.txt", "text/plain", input); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	_, _, first, err := store.Read("file")
	if err != nil || string(first) != "original" {
		t.Fatalf("input mutation corrupted cache: %q %v", first, err)
	}
	first[0] = 'Y'
	_, _, second, err := store.Read("file")
	if err != nil || string(second) != "original" {
		t.Fatalf("reader mutation corrupted cache: %q %v", second, err)
	}
}

func TestMemoryContentStorePrunesExpiredOnWrite(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Hour)
	if err := store.Store("old", "a.txt", "text/plain", []byte("old")); err != nil {
		t.Fatal(err)
	}
	store.entries["old"].expiresAt = time.Now().Add(-time.Hour)
	if err := store.Store("new", "b.txt", "text/plain", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if len(store.entries) != 1 {
		t.Fatalf("expired bytes retained: %d entries", len(store.entries))
	}
}

func TestMemoryContentStoreConcurrentIsolation(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Hour)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			id, data := fmt.Sprint(i), bytes.Repeat([]byte{byte(i)}, 32)
			if err := store.Store(id, "a.txt", "text/plain", data); err != nil {
				t.Error(err)
				return
			}
			for range 10 {
				_, _, read, err := store.Read(id)
				if err != nil || !bytes.Equal(read, data) {
					t.Errorf("corrupted file %s: %v", id, err)
					return
				}
				read[0]++
			}
		})
	}
	wg.Wait()
}

func TestMemoryContentStore_StoreAndRead(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Hour)
	if err := store.Store("file-1", "hello.txt", "text/plain", []byte("hello world")); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	filename, mimeType, data, err := store.Read("file-1")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if filename != "hello.txt" {
		t.Errorf("filename mismatch: got %q", filename)
	}
	if mimeType != "text/plain" {
		t.Errorf("mime type mismatch: got %q", mimeType)
	}
	if string(data) != "hello world" {
		t.Errorf("data mismatch: got %q", string(data))
	}
}

func TestMemoryContentStore_ReadMissing(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Hour)
	_, _, _, err := store.Read("missing")
	if err != ErrFileNotFound {
		t.Fatalf("expected ErrFileNotFound, got %v", err)
	}
}

func TestMemoryContentStore_ReadEmptyID(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Hour)
	_, _, _, err := store.Read("   ")
	if err != ErrFileNotFound {
		t.Fatalf("expected ErrFileNotFound for empty id, got %v", err)
	}
}

func TestMemoryContentStore_TooLarge(t *testing.T) {
	store := NewMemoryContentStore(5, time.Hour)
	if err := store.Store("file-1", "big.txt", "text/plain", []byte("hello world")); err != ErrFileTooLarge {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestMemoryContentStore_Expired(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Nanosecond)
	if err := store.Store("file-1", "hello.txt", "text/plain", []byte("hello world")); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	_, _, _, err := store.Read("file-1")
	if err != ErrFileNotFound {
		t.Fatalf("expected ErrFileNotFound after expiry, got %v", err)
	}
}

func TestMemoryContentStore_CleanupExpired(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Nanosecond)
	if err := store.Store("a", "a.txt", "text/plain", []byte("a")); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	if err := store.Store("b", "b.txt", "text/plain", []byte("b")); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	store.CleanupExpired()
	if _, _, _, err := store.Read("a"); err != ErrFileNotFound {
		t.Errorf("expected a expired")
	}
	if _, _, _, err := store.Read("b"); err != ErrFileNotFound {
		t.Errorf("expected b expired")
	}
}

func TestMemoryContentStore_Delete(t *testing.T) {
	store := NewMemoryContentStore(1024, time.Hour)
	if err := store.Store("file-1", "hello.txt", "text/plain", []byte("hello world")); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	store.Delete("file-1")
	if _, _, _, err := store.Read("file-1"); err != ErrFileNotFound {
		t.Fatalf("expected ErrFileNotFound after delete, got %v", err)
	}
}

func TestMemoryContentStoreBoundsAndReplacementAccounting(t *testing.T) {
	store := NewMemoryContentStore(16, time.Hour)
	store.maxTotalBytes, store.maxEntries = 10, 2
	put := func(id, data string) {
		t.Helper()
		if err := store.Store(id, id+".txt", "text/plain", []byte(data)); err != nil {
			t.Fatal(err)
		}
		actual := 0
		for _, entry := range store.entries {
			actual += len(entry.data)
		}
		if actual != store.totalBytes || actual > 10 || len(store.entries) > 2 {
			t.Fatalf("cache exceeds bounds or miscounts: actual=%d tracked=%d count=%d", actual, store.totalBytes, len(store.entries))
		}
	}
	put("a", "123456")
	store.entries["a"].expiresAt = time.Now().Add(time.Minute)
	put("b", "1234")
	put("c", "12345") // Byte pressure must evict a, nearest expiry.
	if _, _, _, err := store.Read("a"); err != ErrFileNotFound {
		t.Fatal("oldest entry was not evicted")
	}
	put("b", "1") // Replacements must subtract old bytes before accounting.
	if store.totalBytes != 6 {
		t.Fatalf("replacement bytes=%d", store.totalBytes)
	}
	store.Delete("b")
	store.Delete("b")
	if store.totalBytes != 5 {
		t.Fatalf("delete bytes=%d", store.totalBytes)
	}
	store.entries["c"].expiresAt = time.Now().Add(-time.Hour)
	store.CleanupExpired()
	if store.totalBytes != 0 {
		t.Fatal("expired bytes retained in accounting")
	}
	put("empty-a", "")
	put("empty-b", "")
	put("empty-c", "") // Empty files still consume an entry slot.
	if len(store.entries) != 2 {
		t.Fatal("empty entries are unbounded")
	}
	if err := store.Store("oversized", "a", "", []byte("12345678901")); err != ErrFileTooLarge {
		t.Fatalf("total byte limit not applied to single entry: %v", err)
	}
}
