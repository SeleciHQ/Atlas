package connectors

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestNoAtlasInternalImports enforces the connector boundary: adapters must not
// depend on Atlas domain packages (canonical repositories, services, tenancy).
func TestNoAtlasInternalImports(t *testing.T) {
	files, err := parser.ParseDir(token.NewFileSet(), ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse connectors package: %v", err)
	}
	for _, pkg := range files {
		for name, file := range pkg.Files {
			for _, imported := range file.Imports {
				path := strings.Trim(imported.Path.Value, `"`)
				if strings.HasPrefix(path, "atlas/") {
					t.Fatalf("%s imports %s; connectors must stay domain-independent", name, path)
				}
			}
		}
	}
}
