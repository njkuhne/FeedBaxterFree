package ingest

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestIngestHasNoDatabaseImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{"internal/store", "database/sql", "pgx", "sqlite"}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			for _, b := range banned {
				if strings.Contains(p, b) {
					t.Errorf("%s imports %s", n, p)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no files checked")
	}
}
