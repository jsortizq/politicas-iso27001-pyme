package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestCheckFileReferences_AllPresent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs/es"), 0755)
	os.MkdirAll(filepath.Join(dir, "docs/en"), 0755)
	os.WriteFile(filepath.Join(dir, "docs/es/policy.md"), []byte("# Policy"), 0644)
	os.WriteFile(filepath.Join(dir, "docs/en/policy.md"), []byte("# Policy"), 0644)

	controls := []SOAControl{
		{ID: "A.5.1", PolicyFileES: strPtr("docs/es/policy.md"), PolicyFileEN: strPtr("docs/en/policy.md")},
	}

	findings := CheckFileReferences(controls, dir)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
	}
}

func TestCheckFileReferences_Missing(t *testing.T) {
	dir := t.TempDir()

	controls := []SOAControl{
		{ID: "A.5.1", PolicyFileES: strPtr("docs/es/missing.md"), PolicyFileEN: strPtr("docs/en/missing.md")},
	}

	findings := CheckFileReferences(controls, dir)
	if len(findings) != 2 {
		t.Errorf("expected 2 findings for missing files, got %d", len(findings))
	}
}

func TestCheckFileReferences_NullPolicyFiles(t *testing.T) {
	dir := t.TempDir()
	controls := []SOAControl{
		{ID: "A.8.9", PolicyFileES: nil, PolicyFileEN: nil},
	}

	findings := CheckFileReferences(controls, dir)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for null policy files, got %d", len(findings))
	}
}

func TestCheckSummaryConsistency_Correct(t *testing.T) {
	controls := []SOAControl{
		{ID: "A.5.1", ImplementationStatus: "implemented"},
		{ID: "A.8.9", ImplementationStatus: "partial"},
	}

	findings := CheckSummaryConsistency(controls, 2, 1, 1)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for correct summary, got %d", len(findings))
	}
}

func TestCheckSummaryConsistency_Wrong(t *testing.T) {
	controls := []SOAControl{
		{ID: "A.5.1", ImplementationStatus: "implemented"},
		{ID: "A.5.2", ImplementationStatus: "implemented"},
		{ID: "A.8.9", ImplementationStatus: "partial"},
	}

	// Stored says 2 implemented, actual is 2... wait, 2 implemented, but let's test with wrong values
	findings := CheckSummaryConsistency(controls, 3, 3, 0) // says 3 impl, 0 partial — wrong
	if len(findings) != 2 {
		t.Errorf("expected 2 findings (implemented + partial wrong), got %d", len(findings))
	}
}
