package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Error("version should not be empty")
	}
	if v != "1.1.0" {
		t.Errorf("expected 1.1.0, got %s", v)
	}
}

func TestExport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")

	data := map[string]interface{}{
		"results": []string{"ok"},
	}

	err := Export(path, "check", data)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read output: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed["tool"] != "iso-cli" {
		t.Errorf("expected tool=iso-cli, got %v", parsed["tool"])
	}
	if parsed["command"] != "check" {
		t.Errorf("expected command=check, got %v", parsed["command"])
	}
	if parsed["version"] != "1.1.0" {
		t.Errorf("expected version=1.1.0, got %v", parsed["version"])
	}
	if _, ok := parsed["timestamp"]; !ok {
		t.Error("expected timestamp in output")
	}
}
