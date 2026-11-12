// Package soa handles loading, querying and summarizing the Statement of
// Applicability (SoA) matrix for ISO 27001 controls.
package soa

import (
	"encoding/json"
	"os"
)

// Matrix represents the full Statement of Applicability file.
type Matrix struct {
	Metadata Metadata  `json:"metadata"`
	Controls []Control `json:"controls"`
	Summary  Summary   `json:"summary"`
}

// Metadata contains document identification fields.
type Metadata struct {
	Standard     string `json:"standard"`
	Document     string `json:"document"`
	Version      string `json:"version"`
	LastUpdated  string `json:"last_updated"`
	Organization string `json:"organization,omitempty"`
	PreparedBy   string `json:"prepared_by,omitempty"`
	ApprovedBy   string `json:"approved_by,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// Control represents a single control in the SoA matrix.
type Control struct {
	ID                   string  `json:"id"`
	Clause               string  `json:"clause"`
	Category             string  `json:"category"`
	Applicable           bool    `json:"applicable"`
	Justification        string  `json:"justification"`
	ImplementationStatus string  `json:"implementation_status"`
	PolicyFileES         *string `json:"policy_file_es"`
	PolicyFileEN         *string `json:"policy_file_en"`
	ValidationMethod     string  `json:"validation_method"`
	Evidence             string  `json:"evidence,omitempty"`
	Owner                string  `json:"owner"`
}

// Summary contains aggregate statistics computed from the controls.
type Summary struct {
	TotalControls        int     `json:"total_controls_evaluated"`
	Applicable           int     `json:"applicable"`
	NotApplicable        int     `json:"not_applicable"`
	FullyImplemented     int     `json:"fully_implemented"`
	PartiallyImplemented int     `json:"partially_implemented"`
	NotImplemented       int     `json:"not_implemented"`
	CompliancePercentage float64 `json:"compliance_percentage"`
}

// Load reads and parses a SoA matrix from a JSON file.
func Load(path string) (*Matrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m Matrix
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

// ComputeSummary recalculates the summary statistics from the actual
// control records, fixing any inconsistencies in the stored summary.
func ComputeSummary(controls []Control) Summary {
	s := Summary{
		TotalControls: len(controls),
	}

	for _, c := range controls {
		if c.Applicable {
			s.Applicable++
		} else {
			s.NotApplicable++
		}

		switch c.ImplementationStatus {
		case "implemented":
			s.FullyImplemented++
		case "partial":
			s.PartiallyImplemented++
		case "not_implemented":
			s.NotImplemented++
		}
	}

	if s.Applicable > 0 {
		s.CompliancePercentage = float64(s.FullyImplemented) / float64(s.Applicable) * 100
	}

	return s
}

// FilterByStatus returns controls matching the given implementation status.
// An empty status string returns all controls.
func FilterByStatus(controls []Control, status string) []Control {
	if status == "" {
		return controls
	}
	var filtered []Control
	for _, c := range controls {
		if c.ImplementationStatus == status {
			filtered = append(filtered, c)
		}
	}
	return filtered
}
