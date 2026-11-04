package chathistory

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func indexFixture(n int) *Store {
	s := &Store{details: map[string]Entry{}, state: File{Limit: MaxLimit}}
	for i := 0; i < n; i++ {
		id := fmt.Sprint(i)
		s.details[id] = Entry{ID: id, CreatedAt: int64(i / 3), Revision: int64(i), Content: strings.Repeat("long answer ", 100)}
	}
	s.rebuildIndexLocked()
	return s
}
func TestIncrementalHistorySummaryMatchesFullRebuild(t *testing.T) {
	s := indexFixture(MaxLimit)
	for round := 0; round < 3; round++ {
		for i := 0; i < MaxLimit; i++ {
			id := fmt.Sprint(i)
			item := s.details[id]
			item.Revision = int64(100 + round*MaxLimit + i)
			item.Content = fmt.Sprintf("changed-%d-%d", round, i)
			s.details[id] = item
			if !s.updateSummaryLocked(item) {
				t.Fatal("summary not found")
			}
			incremental := append([]SummaryEntry(nil), s.state.Items...)
			s.rebuildIndexLocked()
			if !reflect.DeepEqual(incremental, s.state.Items) {
				t.Fatalf("index diverged at %d/%d", round, i)
			}
		}
	}
	if s.updateSummaryLocked(Entry{ID: "missing"}) {
		t.Fatal("missing summaries need a full rebuild")
	}
}
func BenchmarkHistorySummaryUpdate(b *testing.B) {
	for _, incremental := range []bool{false, true} {
		b.Run(fmt.Sprint(incremental), func(b *testing.B) {
			s := indexFixture(MaxLimit)
			item := s.details["25"]
			item.Revision = 1000
			b.ReportAllocs()
			for b.Loop() {
				item.Revision++
				s.details[item.ID] = item
				if incremental {
					s.updateSummaryLocked(item)
				} else {
					s.rebuildIndexLocked()
				}
			}
		})
	}
}
