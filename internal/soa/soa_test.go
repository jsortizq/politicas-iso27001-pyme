package soa

import (
	"os"
	"path/filepath"
	"testing"
)

const testSOA = `{
  "metadata": {
    "standard": "ISO/IEC 27001:2022",
    "document": "Statement of Applicability",
    "version": "1.0",
    "last_updated": "2024-01-01"
  },
  "controls": [
    {
      "id": "A.5.1",
      "clause": "Policies for information security",
      "category": "Organizational",
      "applicable": true,
      "justification": "Required",
      "implementation_status": "implemented",
      "validation_method": "review",
      "owner": "CISO"
    },
    {
      "id": "A.5.2",
      "clause": "Information security roles",
      "category": "Organizational",
      "applicable": true,
      "justification": "Required",
      "implementation_status": "partial",
      "validation_method": "review",
      "owner": "CISO"
    },
    {
      "id": "A.5.3",
      "clause": "Segregation of duties",
      "category": "Organizational",
      "applicable": false,
      "justification": "Small team",
      "implementation_status": "not_implemented",
      "validation_method": "n/a",
      "owner": "CISO"
    }
  ],
  "summary": {
    "total_controls_evaluated": 3,
    "applicable": 2,
    "not_applicable": 1,
    "fully_implemented": 2,
    "partially_implemented": 0,
    "not_implemented": 1,
    "compliance_percentage": 100
  }
}`

func writeTestSOA(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "SOA_Matrix.json")
	if err := os.WriteFile(path, []byte(testSOA), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	path := writeTestSOA(t)
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if m.Metadata.Standard != "ISO/IEC 27001:2022" {
		t.Errorf("unexpected standard: %s", m.Metadata.Standard)
	}
	if len(m.Controls) != 3 {
		t.Errorf("expected 3 controls, got %d", len(m.Controls))
	}
}

func TestLoad_NotFound(t *testing.T) {
	_, err := Load("/nonexistent/file.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestComputeSummary(t *testing.T) {
	path := writeTestSOA(t)
	m, _ := Load(path)

	s := ComputeSummary(m.Controls)

	if s.TotalControls != 3 {
		t.Errorf("expected 3 total, got %d", s.TotalControls)
	}
	if s.Applicable != 2 {
		t.Errorf("expected 2 applicable, got %d", s.Applicable)
	}
	if s.NotApplicable != 1 {
		t.Errorf("expected 1 not applicable, got %d", s.NotApplicable)
	}
	if s.FullyImplemented != 1 {
		t.Errorf("expected 1 fully implemented, got %d", s.FullyImplemented)
	}
	if s.PartiallyImplemented != 1 {
		t.Errorf("expected 1 partially implemented, got %d", s.PartiallyImplemented)
	}
	if s.NotImplemented != 1 {
		t.Errorf("expected 1 not implemented, got %d", s.NotImplemented)
	}

	// The stored summary says 100% but actual is 1/2 = 50%
	expectedPct := 50.0
	if s.CompliancePercentage != expectedPct {
		t.Errorf("expected %.1f%% compliance, got %.1f%%", expectedPct, s.CompliancePercentage)
	}
}

func TestComputeSummary_DetectsInconsistency(t *testing.T) {
	path := writeTestSOA(t)
	m, _ := Load(path)

	stored := m.Summary
	computed := ComputeSummary(m.Controls)

	// The test data has an intentionally wrong stored summary
	if stored.CompliancePercentage == computed.CompliancePercentage {
		t.Error("stored and computed should differ (test data has intentional inconsistency)")
	}
}

func TestFilterByStatus_All(t *testing.T) {
	path := writeTestSOA(t)
	m, _ := Load(path)

	filtered := FilterByStatus(m.Controls, "")
	if len(filtered) != 3 {
		t.Errorf("expected 3 controls with empty filter, got %d", len(filtered))
	}
}

func TestFilterByStatus_Implemented(t *testing.T) {
	path := writeTestSOA(t)
	m, _ := Load(path)

	filtered := FilterByStatus(m.Controls, "implemented")
	if len(filtered) != 1 {
		t.Errorf("expected 1 implemented control, got %d", len(filtered))
	}
}

func TestFilterByStatus_Partial(t *testing.T) {
	path := writeTestSOA(t)
	m, _ := Load(path)

	filtered := FilterByStatus(m.Controls, "partial")
	if len(filtered) != 1 {
		t.Errorf("expected 1 partial control, got %d", len(filtered))
	}
}

func TestFilterByStatus_NoMatch(t *testing.T) {
	path := writeTestSOA(t)
	m, _ := Load(path)

	filtered := FilterByStatus(m.Controls, "nonexistent_status")
	if len(filtered) != 0 {
		t.Errorf("expected 0 controls for invalid status, got %d", len(filtered))
	}
}
