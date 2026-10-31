package shared

import (
	"crypto/md5"
	"encoding/json"
	"fmt"

	"ds2api/internal/config"
)

func ComputeSyncHash(store ConfigStore) string {
	if store == nil {
		return ""
	}
	snap := store.Snapshot().Clone()
	snap.ClearAccountTokens()
	snap.ClearVercelCredentials()
	snap.VercelSyncHash = ""
	snap.VercelSyncTime = 0
	b, _ := json.Marshal(snap)
	sum := md5.Sum(b)
	return fmt.Sprintf("%x", sum)
}

func SyncHashForJSON(s string) string {
	var cfg config.Config
	if err := json.Unmarshal([]byte(s), &cfg); err != nil {
		return ""
	}
	cfg.VercelSyncHash = ""
	cfg.VercelSyncTime = 0
	cfg.ClearAccountTokens()
	cfg.ClearVercelCredentials()
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	sum := md5.Sum(b)
	return fmt.Sprintf("%x", sum)
}
