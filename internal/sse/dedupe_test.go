package sse

import (
	"fmt"
	"strings"
	"testing"
)

func TestTrimContinuationOverlapSnapshotBoundaries(t *testing.T) {
	for _, char := range []string{"a", "字", "😀"} {
		for _, size := range []int{31, 32, 33} {
			incoming := strings.Repeat(char, size)
			for _, suffix := range []string{"", "more"} {
				t.Run(fmt.Sprintf("%s/%d/%s", char, size, suffix), func(t *testing.T) {
					existing := incoming + suffix
					want := incoming
					if size >= minContinuationSnapshotLen {
						want = ""
					}
					if got := TrimContinuationOverlap(existing, incoming); got != want {
						t.Fatalf("string overlap = %q, want %q", got, want)
					}
					var builder strings.Builder
					builder.WriteString(existing)
					if got := TrimContinuationOverlapFromBuilder(&builder, incoming); got != want {
						t.Fatalf("builder overlap = %q, want %q", got, want)
					}
				})
			}
		}
	}
}

func BenchmarkTrimContinuationOverlapIndependentDelta(b *testing.B) {
	for _, size := range []int{1024, 1 << 20} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			existing := "previous " + strings.Repeat("字", size)
			incoming := "independent " + strings.Repeat("字", size)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				TrimContinuationOverlap(existing, incoming)
			}
		})
	}
}

func TestTrimContinuationOverlapReturnsSuffixForSnapshotReplay(t *testing.T) {
	existing := "我们被问到：这是一个很长的续答快照前缀，用来验证去重逻辑不会误伤正常 token。"
	incoming := existing + "继续分析"
	got := TrimContinuationOverlap(existing, incoming)
	if got != "继续分析" {
		t.Fatalf("expected suffix only, got %q", got)
	}
}

func TestTrimContinuationOverlapDropsStaleShorterSnapshot(t *testing.T) {
	incoming := "我们被问到：这是一个很长的续答快照前缀，用来验证去重逻辑不会误伤正常 token。"
	existing := incoming + "继续分析"
	got := TrimContinuationOverlap(existing, incoming)
	if got != "" {
		t.Fatalf("expected stale snapshot to be dropped, got %q", got)
	}
}

func TestTrimContinuationOverlapPreservesNormalIncrement(t *testing.T) {
	existing := "我们"
	incoming := "被"
	got := TrimContinuationOverlap(existing, incoming)
	if got != "被" {
		t.Fatalf("expected normal increment unchanged, got %q", got)
	}
}

func TestTrimContinuationOverlapKeepsShortPrefixLikeNormalToken(t *testing.T) {
	existing := "我们被问到"
	incoming := "我们"
	got := TrimContinuationOverlap(existing, incoming)
	if got != "我们" {
		t.Fatalf("expected short token preserved, got %q", got)
	}
}

func TestTrimContinuationOverlapKeepsShortMultibyteChunk(t *testing.T) {
	existing := strings.Repeat("字", 36)
	incoming := strings.Repeat("字", 16)
	got := TrimContinuationOverlap(existing, incoming)
	if got != incoming {
		t.Fatalf("expected short multibyte chunk preserved, got %q", got)
	}
}
