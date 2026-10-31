package devid

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ds2api/internal/config"
)

type Readiness struct {
	Ready          bool   `json:"ready"`
	AssetsVerified bool   `json:"assets_verified"`
	NodeVersion    string `json:"node_version,omitempty"`
	Error          string `json:"error,omitempty"`
}

// ReadinessCache bounds probing overhead for the queue-status polling endpoint.
// The cache belongs to a client, so accounts do not share mutable global state.
type ReadinessCache struct {
	mu      sync.Mutex
	key     string
	checked time.Time
	value   Readiness
}

func (c *ReadinessCache) Check(ctx context.Context) Readiness {
	key := strings.Join([]string{LocateScript(), config.DevidDir(), config.DevidStateDir(), os.Getenv("DS2API_DEVID_NODE")}, "\x00")
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key == key && time.Since(c.checked) < 10*time.Second {
		return c.value
	}
	value := checkEnvironment(ctx)
	if ctx.Err() == nil {
		c.key, c.checked, c.value = key, time.Now(), value
	}
	return value
}

func checkEnvironment(ctx context.Context) (result Readiness) {
	source, err := os.ReadFile(LocateScript())
	if err != nil || !strings.Contains(string(source), "DS2API_DEVID_CAPTURE_V1") {
		result.Error = "SDK generator is missing or outdated; use the current repository scripts"
		return result
	}
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	node := strings.TrimSpace(os.Getenv("DS2API_DEVID_NODE"))
	if node == "" {
		node = "node"
	}
	cmd := exec.CommandContext(runCtx, node, LocateScript(), "--selftest", "--timeout", "2")
	cmd.Env = append(os.Environ(), "DS2API_DEVID_DIR="+config.DevidDir())
	output, err := cmd.Output()
	var payload struct {
		OK             bool   `json:"ok"`
		SDKPayload     bool   `json:"sdk_payload"`
		AssetsVerified bool   `json:"assets_verified"`
		NodeVersion    string `json:"node_version"`
		Error          string `json:"error"`
	}
	if parseErr := json.Unmarshal([]byte(lastJSONLine(string(output))), &payload); parseErr == nil {
		result.Error, result.NodeVersion, result.AssetsVerified = payload.Error, payload.NodeVersion, payload.AssetsVerified
		if err == nil && payload.OK && payload.SDKPayload {
			dir := filepath.Join(config.DevidStateDir(), "profiles")
			if err = os.MkdirAll(dir, 0o700); err == nil {
				var file *os.File
				if file, err = os.CreateTemp(dir, ".write-check-*"); err == nil {
					err = errors.Join(file.Close(), os.Remove(file.Name()))
				}
			}
			result.Ready = err == nil
		}
	}
	if result.Error == "" && !result.Ready {
		if err != nil {
			result.Error = "SDK runtime unavailable: " + err.Error()
		} else {
			result.Error = "SDK generator returned an invalid readiness response"
		}
	}
	return result
}
