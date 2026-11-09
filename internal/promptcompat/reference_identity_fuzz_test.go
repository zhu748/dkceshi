package promptcompat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzReferenceAliasesKeepOneIdentity(f *testing.F) {
	f.Add("same", "same")
	f.Add("case-A", "case-a")
	f.Add("中文\r\n", "中文\r\n")
	f.Fuzz(func(t *testing.T, left, right string) {
		if len(left)+len(right) > 4096 || !utf8.ValidString(left) || !utf8.ValidString(right) {
			return
		}
		first, second := "file-"+strings.TrimSpace(left), "file-"+strings.TrimSpace(right)
		request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "BODY", "files": []any{
			map[string]any{"id": first, "file": map[string]any{"file_id": second}, "filename": "source.txt"},
		}}}}
		before, _ := json.Marshal(request)
		standard, err := NormalizeOpenAIChatRequest(nil, request, "")
		if first != second {
			if err == nil {
				t.Fatal("conflicting file identities were accepted")
			}
		} else {
			if err != nil || !reflect.DeepEqual(standard.RefFileIDs, []string{first}) {
				t.Fatal("equivalent aliases did not keep one identity", err)
			}
			bridge, err := NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(request), "")
			if err != nil || !reflect.DeepEqual(standard.Messages, bridge.Messages) || !reflect.DeepEqual(bridge.RefFileIDs, []string{first}) {
				t.Fatal("internal Chat hop changed reference identity", err)
			}
		}
		after, _ := json.Marshal(request)
		if string(before) != string(after) {
			t.Fatal("reference normalization mutated source data")
		}
	})
}
