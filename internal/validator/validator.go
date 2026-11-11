// Package validator checks Markdown documentation for unfilled template
// variables such as [EMPRESA] or [NOMBRE_RESPONSABLE].
package validator

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding represents a single unfilled template variable in a document.
type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Variable string `json:"variable"`
	Context  string `json:"context"`
}

// knownNonVariables lists acronyms that look like template variables
// but are legitimate content in security documentation.
var knownNonVariables = map[string]bool{
	"MIT": true, "RFC": true, "ISO": true, "IEC": true,
	"DNS": true, "SPF": true, "DKIM": true, "DMARC": true,
	"AES": true, "SSL": true, "TLS": true, "VPN": true,
	"HTTPS": true, "HTTP": true, "SSH": true, "SMTP": true,
	"RTO": true, "RPO": true, "MFA": true, "BYOD": true,
	"TODO": true, "NOTE": true, "WARNING": true,
	"SGSI": true, "ISMS": true, "SoA": true,
}

var varPattern = regexp.MustCompile(`\[([A-Z][A-Z0-9_]{2,})\]`)

// ValidateDocs scans all Markdown files under root for unfilled template
// variables. Returns the findings and the total number of files scanned.
func ValidateDocs(root string) ([]Finding, int, error) {
	var findings []Finding
	fileCount := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		fileCount++
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			matches := varPattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if knownNonVariables[match[1]] {
					continue
				}
				findings = append(findings, Finding{
					File:     path,
					Line:     lineNum + 1,
					Variable: match[0],
					Context:  strings.TrimSpace(line),
				})
			}
		}

		return nil
	})

	return findings, fileCount, err
}
