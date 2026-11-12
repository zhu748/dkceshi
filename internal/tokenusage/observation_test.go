package tokenusage

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestSharedUpstreamUsageFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../tests/fixtures/upstream-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string
		Chunk    map[string]any
		Expected Observation
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if got := Parse(tc.Chunk); !reflect.DeepEqual(got, tc.Expected) {
				t.Fatalf("got %+v, expected %+v", got, tc.Expected)
			}
		})
	}
}

func TestSnapshotsRetainLatestCumulativeWithoutAddingOrAliasing(t *testing.T) {
	var snapshot Snapshot
	for _, raw := range []string{
		`{"v":{"response":{"message_id":2,"accumulated_token_usage":0}}}`,
		`{"p":"response/accumulated_token_usage","v":2413}`,
		`{"v":{"response":{"message_id":2,"accumulated_token_usage":2413}}}`,
		`{"p":"response/accumulated_token_usage","v":2551}`,
		`{"p":"response/accumulated_token_usage","v":2413}`,
	} {
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatal(err)
		}
		snapshot.Observe(Parse(m))
	}
	if len(snapshot.Messages) != 1 || *snapshot.Messages[0].AccumulatedTokens != 2551 {
		t.Fatalf("cumulative values were summed or regressed: %+v", snapshot)
	}
	copy := snapshot.Clone()
	*copy.Messages[0].AccumulatedTokens = 7
	if *snapshot.Messages[0].AccumulatedTokens != 2551 {
		t.Fatal("snapshot shares mutable counts")
	}
	for id := 3; id < 80; id++ {
		snapshot.Observe(Observation{MessageID: id, AccumulatedTokens: countPointer(id)})
	}
	if len(snapshot.Messages) != MaxMessages || !snapshot.Truncated {
		t.Fatal("unbounded metadata")
	}
}
