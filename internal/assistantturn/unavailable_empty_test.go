package assistantturn

import "testing"

func TestUnavailableEmptyPolicyOnlyMatchesKnownNoOutput503(t *testing.T) {
	for _, tc := range []struct {
		err  *OutputError
		want bool
	}{
		{nil, false},
		{&OutputError{Status: 503, Code: "upstream_unavailable", Message: UpstreamUnavailableEmptyMessage}, true},
		{&OutputError{Status: 503, Code: "upstream_unavailable", Message: "Failed to get completion."}, false},
		{&OutputError{Status: 503, Code: "upstream_unavailable", Message: "Service Unavailable"}, false},
		{&OutputError{Status: 400, Code: "content_filter", Message: UpstreamUnavailableEmptyMessage}, false},
		{&OutputError{Status: 502, Code: "upstream_read_error", Message: UpstreamUnavailableEmptyMessage}, false},
	} {
		if got := IsUnavailableEmptyOutput(tc.err); got != tc.want {
			t.Fatalf("error=%+v got=%v want=%v", tc.err, got, tc.want)
		}
	}
}
