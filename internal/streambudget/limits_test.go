package streambudget

import (
	"bufio"
	"errors"
	"strings"
	"testing"
)

func TestReadLineStopsAtLimitBeforeCallback(t *testing.T) {
	for _, source := range []string{strings.Repeat("x", 10000), strings.Repeat("x", 10000) + "\n"} {
		line, err := ReadLine(bufio.NewReaderSize(strings.NewReader(source), 32), 100)
		if !errors.Is(err, ErrLimit) || line != nil {
			t.Fatalf("oversized line accepted: %d %v", len(line), err)
		}
	}
	line, err := ReadLine(bufio.NewReader(strings.NewReader("exact\n")), 6)
	if err != nil || string(line) != "exact\n" {
		t.Fatalf("boundary line lost: %q %v", line, err)
	}
}

func TestEnvironmentLimitsArePositiveAndBounded(t *testing.T) {
	for _, raw := range []string{"0", "-1", "invalid", "1073741825"} {
		t.Setenv("DS2API_STREAM_MAX_TOOL_CALLS", raw)
		if Value("max_tool_calls") != 128 {
			t.Fatal("invalid setting escaped default")
		}
	}
	t.Setenv("DS2API_STREAM_MAX_TOOL_CALLS", "3")
	if Value("max_tool_calls") != 3 {
		t.Fatal("valid override ignored")
	}
}
