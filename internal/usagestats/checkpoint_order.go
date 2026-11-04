package usagestats

import "sort"

// Keep the original map keys alongside the snapshot. They share existing
// string bytes; the cache costs one slice of headers, without copying IDs.
type checkpointSorter struct{ payload *fileData }

func (s checkpointSorter) Len() int { return len(s.payload.Entries) }
func (s checkpointSorter) Less(i, j int) bool {
	a, b := s.payload.Entries[i], s.payload.Entries[j]
	if a.Date != b.Date {
		return a.Date < b.Date
	}
	if a.Model != b.Model {
		return a.Model < b.Model
	}
	return entryIdentityLess(a, b)
}
func (s checkpointSorter) Swap(i, j int) {
	s.payload.Entries[i], s.payload.Entries[j] = s.payload.Entries[j], s.payload.Entries[i]
	if len(s.payload.keys) == len(s.payload.Entries) {
		s.payload.keys[i], s.payload.keys[j] = s.payload.keys[j], s.payload.keys[i]
	}
}

func sortCheckpoint(payload *fileData) {
	if !payload.ordered {
		sort.Sort(checkpointSorter{payload})
	}
}
