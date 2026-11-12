package assistantturn

import "ds2api/internal/tokenusage"

func (a *Accumulator) UsageSnapshot() tokenusage.Snapshot {
	if a == nil {
		return tokenusage.Snapshot{}
	}
	return a.inner.UpstreamUsage.Clone()
}
