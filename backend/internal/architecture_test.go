// Package internal_test enforces the dependency rules documented in
// architecture.md §6.3 using only the Go parser (no external tooling).
package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleBoundaries(t *testing.T) {
	transportPackages := map[string]bool{
		"atlas/internal/httpapi": true,
		"atlas/internal/mcpapi":  true,
	}

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != "." && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		directory := filepath.ToSlash(filepath.Dir(path))

		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)

			if strings.HasPrefix(importPath, "atlas/cmd/") {
				t.Errorf("%s imports %s: internal packages must not depend on cmd", path, importPath)
			}

			if transportPackages[importPath] {
				isTransport := directory == "httpapi" || directory == "mcpapi"
				if !isTransport {
					t.Errorf("%s imports transport package %s: only transport packages may depend on it", path, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal packages: %v", err)
	}
}
