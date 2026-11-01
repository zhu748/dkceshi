package textclean

import "regexp"

var leakedThinkTagPattern = regexp.MustCompile(`(?is)</?\s*think\s*>`)

// leakedBOSMarkerPattern matches DeepSeek BOS markers with halfwidth or
// legacy U+FF5C fullwidth delimiters:
//   - ASCII underscore: <|begin_of_sentence|>
//   - U+2581 variant:   <|begin▁of▁sentence|>
var leakedBOSMarkerPattern = regexp.MustCompile(`(?i)<[\|\x{ff5c}]\s*begin[_▁]of[_▁]sentence\s*[\|\x{ff5c}]>`)

// leakedThoughtMarkerPattern matches leaked thought control markers in both
// explicit and compact forms:
//   - ASCII underscore: <| of_thought |>, <| begin_of_thought |>
//   - U+2581 variant:   <|▁of▁thought|>, <|begin▁of▁thought|>
var leakedThoughtMarkerPattern = regexp.MustCompile(`(?i)<[\|\x{ff5c}]\s*(?:begin[_▁])?[_▁]*of[_▁]thought\s*[\|\x{ff5c}]>`)

// leakedMetaMarkerPattern matches the remaining DeepSeek special tokens with
// halfwidth or legacy U+FF5C fullwidth delimiters:
//   - ASCII underscore: <|end_of_sentence|>, <|end_of_toolresults|>, <|end_of_instructions|>
//   - U+2581 variant:   <|end▁of▁sentence|>, <|end▁of▁toolresults|>, <|end▁of▁instructions|>
var leakedMetaMarkerPattern = regexp.MustCompile(`(?i)<[\|\x{ff5c}]\s*(?:assistant|tool|end[_▁]of[_▁]sentence|end[_▁]of[_▁]thinking|end[_▁]of[_▁]thought|end[_▁]of[_▁]toolresults|end[_▁]of[_▁]instructions)\s*[\|\x{ff5c}]>`)

func cleanControlToken(text string) string {
	for _, pattern := range []*regexp.Regexp{leakedThinkTagPattern, leakedBOSMarkerPattern, leakedThoughtMarkerPattern, leakedMetaMarkerPattern} {
		text = pattern.ReplaceAllString(text, "")
	}
	return text
}
