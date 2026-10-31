package compat

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Shared request/output behavior must be usable without importing a protocol's
// HTTP surface. This guard prevents helpers from drifting back into OpenAI.
func TestSharedBusinessPackagesDoNotImportHTTPSurfaces(t *testing.T) {
	for _, pkg := range []string{
		"assistantturn", "completionruntime", "contextfile", "textclean",
		"promptcompat", "prompt", "toolcall", "toolstream", "sse", "stream",
	} {
		t.Run(pkg, func(t *testing.T) {
			dir := filepath.Join("..", pkg)
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
					continue
				}
				name := filepath.Join(dir, file.Name())
				parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
				if err != nil {
					t.Fatal(err)
				}
				for _, imp := range parsed.Imports {
					path, err := strconv.Unquote(imp.Path.Value)
					if err != nil {
						t.Fatal(err)
					}
					if path == "ds2api/internal/httpapi" || strings.HasPrefix(path, "ds2api/internal/httpapi/") {
						t.Errorf("%s imports HTTP surface %s; shared behavior belongs below protocol adapters", name, path)
					}
				}
			}
		})
	}
}
