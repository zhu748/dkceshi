//go:build !386 && !arm && !mips && !mipsle && !wasm

package util

import (
	"strings"
	"testing"
)

func TestTokenizerEncodingForCountCachesSupportedModel(t *testing.T) {
	encoding, release := tokenizerEncodingForCount(defaultTokenizerModel)
	if encoding == nil {
		t.Fatalf("expected tokenizer encoding for %q", defaultTokenizerModel)
	}
	release()

	if _, ok := tokenEncodingPools.Load(defaultTokenizerModel); !ok {
		t.Fatalf("expected tokenizer encoding pool for %q", defaultTokenizerModel)
	}

	encoding, release = tokenizerEncodingForCount(defaultTokenizerModel)
	if encoding == nil {
		t.Fatalf("expected cached tokenizer encoding for %q", defaultTokenizerModel)
	}
	release()
}

func TestTokenizerEncodingForCountCachesUnsupportedModel(t *testing.T) {
	const model = "__ds2api_unsupported_tokenizer_model__"
	encoding, release := tokenizerEncodingForCount(model)
	release()
	if encoding != nil {
		t.Fatalf("expected nil encoding for unsupported model %q", model)
	}
	if _, ok := tokenEncodingUnsupported.Load(model); !ok {
		t.Fatalf("expected unsupported tokenizer model to be cached")
	}
}

func TestTokenCountTreatsSpecialMarkersAsLiteralText(t *testing.T) {
	for _, model := range []string{defaultTokenizerModel, claudeTokenizerModel} {
		encoding, release := tokenizerEncodingForCount(model)
		if encoding == nil {
			// The dependency does not expose every model through its factory;
			// retain the existing heuristic fallback for unsupported models.
			release()
			if countWithTokenizer("中文 <|endoftext|>", model) != 0 {
				t.Fatalf("unsupported tokenizer fallback changed for %s", model)
			}
			continue
		}
		for _, text := range []string{
			"", "  \n\t", "ordinary source code", "中文🙂e\u0301\n文字",
			"literal <|endoftext|> <|fim_prefix|> <|im_start|> <EOT> markers",
			strings.Repeat("<|endoftext|> 中文🙂 ", 32),
			"invalid UTF-8: \xff",
		} {
			trimmed := strings.TrimSpace(text)
			ids, _, err := encoding.Encode(trimmed, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := countWithTokenizer(text, model); got != len(ids) {
				t.Fatalf("model=%s text=%q got=%d want=%d", model, text, got, len(ids))
			}
		}
		release()
	}
}
