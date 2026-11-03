package completionruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"ds2api/internal/tooldiagnostics"
)

type repairBackoffKey struct{}
type repairBackoffEntry struct {
	until time.Time
	err   *repairAPIError
}

var repairBackoffs = struct {
	sync.Mutex
	entries map[[32]byte]repairBackoffEntry
}{entries: map[[32]byte]repairBackoffEntry{}}

func repairConfigurationKey(settings tooldiagnostics.RepairSettings) [32]byte {
	raw, _ := json.Marshal(settings)
	return sha256.Sum256(raw)
}

func pausedRepairConfiguration(ctx context.Context, settings tooldiagnostics.RepairSettings) error {
	if bypass, _ := ctx.Value(repairBackoffKey{}).(bool); bypass {
		return nil
	}
	key := repairConfigurationKey(settings)
	repairBackoffs.Lock()
	defer repairBackoffs.Unlock()
	entry, ok := repairBackoffs.entries[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.until) {
		delete(repairBackoffs.entries, key)
		return nil
	}
	return &repairAPIError{status: entry.err.status, message: fmt.Sprintf("custom repair API configuration paused briefly after HTTP %d: %s", entry.err.status, entry.err.message)}
}

func rememberRepairConfigurationFailure(ctx context.Context, settings tooldiagnostics.RepairSettings, err error) {
	if bypass, _ := ctx.Value(repairBackoffKey{}).(bool); bypass {
		return
	}
	var apiError *repairAPIError
	if !errors.As(err, &apiError) {
		return
	}
	switch apiError.status {
	case 401, 403, 404, 405:
	default:
		return
	}
	repairBackoffs.Lock()
	defer repairBackoffs.Unlock()
	// A bounded cache keeps different gateways/credentials isolated. Keys are
	// one-way digests; no credentials or prompts are stored here.
	if len(repairBackoffs.entries) >= 128 {
		for key, entry := range repairBackoffs.entries {
			if time.Now().After(entry.until) {
				delete(repairBackoffs.entries, key)
			}
		}
		if len(repairBackoffs.entries) >= 128 {
			for key := range repairBackoffs.entries {
				delete(repairBackoffs.entries, key)
				break
			}
		}
	}
	repairBackoffs.entries[repairConfigurationKey(settings)] = repairBackoffEntry{until: time.Now().Add(15 * time.Second), err: apiError}
}

func clearRepairConfigurationFailure(settings tooldiagnostics.RepairSettings) {
	key := repairConfigurationKey(settings)
	repairBackoffs.Lock()
	delete(repairBackoffs.entries, key)
	repairBackoffs.Unlock()
}
