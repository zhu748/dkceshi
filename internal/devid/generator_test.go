package devid

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func generatorFixture(t *testing.T, source string) Options {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for generator subprocess tests")
	}
	script := filepath.Join(t.TempDir(), "设备 生成.cjs")
	if err := os.WriteFile(script, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return Options{Node: node, Script: script, Timeout: 10 * time.Second}
}

func TestGeneratePreservesJSONErrorFromFailedProcess(t *testing.T) {
	opts := generatorFixture(t, `console.log(JSON.stringify({ok:false,error:"SDK 资源不完整: profiles.json"})); process.exit(1)`)
	_, err := Generate(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "SDK 资源不完整: profiles.json") {
		t.Fatalf("lost structured error from stdout: %v", err)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("lost process failure: %v", err)
	}
}

func TestGenerateKeepsStderrWhenNoStructuredError(t *testing.T) {
	opts := generatorFixture(t, `console.error("Cannot find module runtime"); process.exit(1)`)
	_, err := Generate(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "Cannot find module runtime") {
		t.Fatalf("lost bootstrap error: %v", err)
	}
}

func TestGenerateExplainsSilentProcessFailure(t *testing.T) {
	opts := generatorFixture(t, `process.exit(1)`)
	_, err := Generate(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "--selftest") {
		t.Fatalf("silent failure has no diagnostic action: %v", err)
	}
}

func TestGenerateHonorsConfiguredAssetDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DS2API_DEVID_DIR", dir)
	opts := generatorFixture(t, `console.log(JSON.stringify({ok:true,device_id:process.env.DS2API_DEVID_DIR}))`)
	id, err := Generate(context.Background(), opts)
	if err != nil || id != dir {
		t.Fatalf("asset directory mismatch: id=%q err=%v", id, err)
	}
}

func TestGeneratePreservesCancellation(t *testing.T) {
	opts := generatorFixture(t, `setInterval(() => {}, 1000)`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Generate(ctx, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
