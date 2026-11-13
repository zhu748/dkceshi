package keepalive

import (
	"ds2api/internal/config"
	"testing"
	"time"
)

func TestNonFiniteKeepAliveIntervalsCannotPanicOrScheduleRapidRequests(t *testing.T) {
	for _, v := range []string{"NaN", "nan", "NaN-3", "3-NaN", "+Inf", "1-+Inf"} {
		t.Run(v, func(t *testing.T) {
			if config.ValidateKeepAliveInterval(v) == nil {
				t.Fatal("accepted non-finite interval")
			}
			delay := RandomDurationFromInterval(v)
			if delay < 3*time.Minute || delay > 720*time.Hour {
				t.Fatalf("unsafe delay: %s", delay)
			}
		})
	}
}
