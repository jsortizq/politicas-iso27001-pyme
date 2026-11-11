package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDocs_Clean(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "policy.md"), []byte("# Security Policy\nAll systems must comply.\n"), 0644)

	findings, count, err := ValidateDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 file scanned, got %d", count)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestValidateDocs_UnfilledVariables(t *testing.T) {
	dir := t.TempDir()
	content := "# Policy for [EMPRESA]\nResponsible: [NOMBRE_RESPONSABLE]\n"
	os.WriteFile(filepath.Join(dir, "template.md"), []byte(content), 0644)

	findings, _, err := ValidateDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(findings))
	}
}

func TestValidateDocs_KnownAcronyms(t *testing.T) {
	dir := t.TempDir()
	content := "Compliant with [ISO] and [RFC] standards. SGSI [SGSI] scope.\n"
	os.WriteFile(filepath.Join(dir, "standards.md"), []byte(content), 0644)

	findings, _, err := ValidateDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (known acronyms), got %d", len(findings))
	}
}

func TestValidateDocs_IgnoresNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"key": "[EMPRESA]"}`), 0644)
	os.WriteFile(filepath.Join(dir, "clean.md"), []byte("# Clean\n"), 0644)

	_, count, err := ValidateDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 file scanned (only .md), got %d", count)
	}
}

func TestValidateDocs_Subdirectories(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "nested.md"), []byte("[EMPRESA] policy\n"), 0644)

	findings, count, err := ValidateDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 file, got %d", count)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestValidateDocs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	findings, count, err := ValidateDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(findings) != 0 {
		t.Errorf("expected 0 files and 0 findings for empty dir")
	}
}
