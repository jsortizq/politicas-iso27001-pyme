// Package validator — integrity checks for SOA references and document structure.

package validator

import (
	"fmt"
	"os"
	"path/filepath"
)

// IntegrityFinding represents a missing or broken reference.
type IntegrityFinding struct {
	Control    string `json:"control"`
	Field      string `json:"field"`
	Path       string `json:"path"`
	Problem    string `json:"problem"`
}

// SOAControl is the minimal control structure needed for integrity checks.
type SOAControl struct {
	ID               string  `json:"id"`
	PolicyFileES     *string `json:"policy_file_es"`
	PolicyFileEN     *string `json:"policy_file_en"`
	ImplementationStatus string `json:"implementation_status"`
	ValidationMethod string  `json:"validation_method"`
}

// CheckFileReferences verifies that all policy files referenced in controls
// actually exist on disk, relative to repoRoot.
func CheckFileReferences(controls []SOAControl, repoRoot string) []IntegrityFinding {
	var findings []IntegrityFinding

	for _, c := range controls {
		if c.PolicyFileES != nil && *c.PolicyFileES != "" {
			path := filepath.Join(repoRoot, *c.PolicyFileES)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				findings = append(findings, IntegrityFinding{
					Control: c.ID,
					Field:   "policy_file_es",
					Path:    *c.PolicyFileES,
					Problem: "file not found",
				})
			}
		}

		if c.PolicyFileEN != nil && *c.PolicyFileEN != "" {
			path := filepath.Join(repoRoot, *c.PolicyFileEN)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				findings = append(findings, IntegrityFinding{
					Control: c.ID,
					Field:   "policy_file_en",
					Path:    *c.PolicyFileEN,
					Problem: "file not found",
				})
			}
		}
	}

	return findings
}

// CheckSummaryConsistency verifies that the stored summary matches
// what the actual control records say.
func CheckSummaryConsistency(controls []SOAControl, storedTotal, storedImplemented, storedPartial int) []IntegrityFinding {
	var findings []IntegrityFinding

	actualTotal := len(controls)
	actualImplemented := 0
	actualPartial := 0

	for _, c := range controls {
		switch c.ImplementationStatus {
		case "implemented":
			actualImplemented++
		case "partial":
			actualPartial++
		}
	}

	if storedTotal != actualTotal {
		findings = append(findings, IntegrityFinding{
			Problem: fmt.Sprintf("summary.total_controls_evaluated = %d, actual = %d", storedTotal, actualTotal),
		})
	}
	if storedImplemented != actualImplemented {
		findings = append(findings, IntegrityFinding{
			Problem: fmt.Sprintf("summary.fully_implemented = %d, actual = %d", storedImplemented, actualImplemented),
		})
	}
	if storedPartial != actualPartial {
		findings = append(findings, IntegrityFinding{
			Problem: fmt.Sprintf("summary.partially_implemented = %d, actual = %d", storedPartial, actualPartial),
		})
	}

	return findings
}
