package shared

import "testing"

func TestPageBoundsAtIntegerLimits(t *testing.T) {
	maximum := int(^uint(0) >> 1)
	for _, sample := range []struct{ page, size, total, start, end int }{
		{maximum, 10, 1, 1, 1}, {maximum, maximum, 20, 20, 20},
		{1, 10, 0, 0, 0}, {0, 10, 20, 0, 10}, {2, 10, 11, 10, 11},
		{maximum, 1, maximum, maximum - 1, maximum},
	} {
		start, end := PageBounds(sample.page, sample.size, sample.total)
		if start != sample.start || end != sample.end {
			t.Fatalf("%+v: got [%d:%d]", sample, start, end)
		}
	}
}
