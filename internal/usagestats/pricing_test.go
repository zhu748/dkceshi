package usagestats

import "testing"

func TestPriceMatchingPrecedenceAndBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		models      map[string]ModelPrice
		want        float64
		matched     bool
	}{
		{"longest", "prefix-a-b-c-tail", map[string]ModelPrice{"a-b": {InputPrice: 1}, "b-c": {InputPrice: 2}, "a-b-c": {InputPrice: 3}}, 3, true},
		{"lexical tie", "a-b-c", map[string]ModelPrice{"a-b": {InputPrice: 1}, "b-c": {InputPrice: 2}}, 1, true},
		{"exact zero", "A-B-C", map[string]ModelPrice{"a-b-c": {}, "a-b": {InputPrice: 1}}, 0, true},
		{"normalized tie", "prefix-flash-tail", map[string]ModelPrice{" FLASH ": {InputPrice: 1}, "flash": {InputPrice: 2}}, 1, true},
		{"embedded substring", "flashlight", map[string]ModelPrice{"flash": {InputPrice: 1}}, 0, false},
		{"empty key", "flash", map[string]ModelPrice{" ": {InputPrice: 1}}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Repeated calls cover randomized map iteration for equal matches.
			for range 100 {
				price, matched := matchModelPrice(tc.models, tc.model)
				if price.InputPrice != tc.want || matched != tc.matched {
					t.Fatalf("got %+v, %v; want input price %v, %v", price, matched, tc.want, tc.matched)
				}
			}
		})
	}
}
