package usagestats

import "testing"

func BenchmarkMatchModelPrice(b *testing.B) {
	models := map[string]ModelPrice{
		"deepseek": {InputPrice: 1}, "v4.1": {InputPrice: 2},
		"flash": {InputPrice: 3}, "v4.1-flash": {InputPrice: 4},
		"deepseek-v4.1-flash": {InputPrice: 5},
	}
	b.ReportAllocs()
	for b.Loop() {
		price, ok := matchModelPrice(models, "deepseek-v4.1-flash-search-nothinking")
		if !ok || price.InputPrice != 5 {
			b.Fatal(price, ok)
		}
	}
}
