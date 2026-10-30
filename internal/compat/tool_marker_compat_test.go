package compat

import (
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/toolcall"
)

// Both runtimes replay these exact chunks and expectations. Keep literal data
// and buffering behavior consistent between Go and the Vercel Node stream.
func TestGoCompatToolMarkerFixtures(t *testing.T) {
	files, err := filepath.Glob(compatPath("fixtures", "tool_marker", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("load marker fixtures: files=%d err=%v", len(files), err)
	}
	for _, fixturePath := range files {
		name := trimExt(filepath.Base(fixturePath))
		t.Run(name, func(t *testing.T) {
			var fixture struct {
				Marker string   `json:"marker"`
				Chunks []string `json:"chunks"`
			}
			var expected struct {
				Writes []string `json:"writes"`
				Flush  string   `json:"flush"`
				Output string   `json:"output"`
			}
			mustLoadJSON(t, fixturePath, &fixture)
			mustLoadJSON(t, compatPath("expected", "tool_marker_"+name+".json"), &expected)
			if len(fixture.Chunks) != len(expected.Writes) {
				t.Fatal("fixture chunk and expectation counts differ")
			}
			n := toolcall.NewMarkerNormalizer(fixture.Marker)
			var output strings.Builder
			for i, chunk := range fixture.Chunks {
				got := n.Write(chunk)
				if got != expected.Writes[i] {
					t.Fatalf("chunk %d: got %q want %q", i, got, expected.Writes[i])
				}
				output.WriteString(got)
			}
			flushed := n.Flush()
			output.WriteString(flushed)
			if flushed != expected.Flush || output.String() != expected.Output {
				t.Fatalf("got flush=%q output=%q, want flush=%q output=%q", flushed, output.String(), expected.Flush, expected.Output)
			}
		})
	}
}
