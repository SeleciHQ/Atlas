package mcpapi

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateSnapshots = flag.Bool("update", false, "update MCP tool schema snapshots")

type toolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Scope       string         `json:"scope"`
	InputSchema map[string]any `json:"inputSchema"`
}

func TestToolSchemasSnapshot(t *testing.T) {
	tools := newTools(Deps{})
	descriptors := make([]toolDescriptor, 0, len(tools))
	for _, tool := range tools {
		descriptors = append(descriptors, toolDescriptor{
			Name: tool.name, Description: tool.description, Scope: tool.scope, InputSchema: tool.inputSchema,
		})
	}
	encoded, err := json.MarshalIndent(descriptors, "", "  ")
	if err != nil {
		t.Fatalf("encode tool schemas: %v", err)
	}
	encoded = append(encoded, '\n')

	path := filepath.Join("testdata", "tools.json")
	if *updateSnapshots {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		return
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot (run go test ./internal/mcpapi -update): %v", err)
	}
	if string(expected) != string(encoded) {
		t.Fatalf("MCP tool schema snapshot changed; run: go test ./internal/mcpapi -run TestToolSchemasSnapshot -update")
	}
}

func TestParseResourceURI(t *testing.T) {
	organizationID, kind, id, ok := parseResourceURI("atlas://organizations/org-1/invoices/inv-1")
	if !ok || organizationID != "org-1" || kind != "invoices" || id != "inv-1" {
		t.Fatalf("parsed = %s %s %s %v", organizationID, kind, id, ok)
	}
	for _, uri := range []string{
		"https://example.com/invoices/inv-1",
		"atlas://organizations/org-1/invoices",
		"atlas://organizations/org-1/invoices/",
		"atlas://organizations//invoices/inv-1",
	} {
		if _, _, _, ok := parseResourceURI(uri); ok {
			t.Fatalf("uri %q should be rejected", uri)
		}
	}
}
