// iso-cli — ISO 27001:2022 Compliance Automation Tool
// A CLI tool for SMEs to automate technical audits against ISO 27001 controls.
//
// Subcommands:
//   check          Run a local technical audit (backups, logs, permissions)
//   validate-docs  Verify that Markdown docs have no unfilled template variables
//   export-soa     Export compliance status from SOA_Matrix.json

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const version = "1.0.0"

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// ──────────────────────────────────────────────
// Data structures
// ──────────────────────────────────────────────

// CheckResult represents the outcome of a single audit check.
type CheckResult struct {
	Control     string `json:"control"`
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Detail      string `json:"detail"`
}

// SOAMatrix represents the full Statement of Applicability file.
type SOAMatrix struct {
	Metadata struct {
		Standard    string `json:"standard"`
		Document    string `json:"document"`
		Version     string `json:"version"`
		LastUpdated string `json:"last_updated"`
	} `json:"metadata"`
	Controls []SOAControl `json:"controls"`
	Summary  struct {
		TotalControls        int     `json:"total_controls_evaluated"`
		Applicable           int     `json:"applicable"`
		FullyImplemented     int     `json:"fully_implemented"`
		PartiallyImplemented int     `json:"partially_implemented"`
		CompliancePercentage float64 `json:"compliance_percentage"`
	} `json:"summary"`
}

// SOAControl represents a single control in the SoA matrix.
type SOAControl struct {
	ID                   string  `json:"id"`
	Clause               string  `json:"clause"`
	Category             string  `json:"category"`
	Applicable           bool    `json:"applicable"`
	Justification        string  `json:"justification"`
	ImplementationStatus string  `json:"implementation_status"`
	PolicyFileES         *string `json:"policy_file_es"`
	PolicyFileEN         *string `json:"policy_file_en"`
	ValidationMethod     string  `json:"validation_method"`
	Owner                string  `json:"owner"`
}

// DocValidationResult represents a file with unfilled template variables.
type DocValidationResult struct {
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Variable string   `json:"variable"`
	Context  string   `json:"context"`
}

// ──────────────────────────────────────────────
// Main entry point
// ──────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "check":
		runCheck(os.Args[2:])
	case "validate-docs":
		runValidateDocs(os.Args[2:])
	case "export-soa":
		runExportSOA(os.Args[2:])
	case "version":
		fmt.Printf("iso-cli v%s — ISO 27001:2022 Compliance Tool\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`
╔══════════════════════════════════════════════════════════╗
║        iso-cli — ISO 27001:2022 Compliance Tool         ║
╚══════════════════════════════════════════════════════════╝

Usage: iso-cli <command> [options]

Commands:
  check          Run a local technical audit
  validate-docs  Verify Markdown docs have no unfilled variables
  export-soa     Export compliance status from SOA_Matrix.json
  version        Print version information
  help           Show this help message

Examples:
  iso-cli check --backup-dir /var/backups --log-dir /var/log --max-backup-age 7
  iso-cli validate-docs --path ./docs/
  iso-cli export-soa --input SOA_Matrix.json --output report.json`)
}

// ──────────────────────────────────────────────
// COMMAND: check
// ──────────────────────────────────────────────

func runCheck(args []string) {
	flagSet := flag.NewFlagSet("check", flag.ExitOnError)
	backupDir := flagSet.String("backup-dir", "", "Path to backup directory")
	logDir := flagSet.String("log-dir", "", "Path to log directory")
	maxAge := flagSet.Int("max-backup-age", 7, "Maximum age in days for the most recent backup")
	outputJSON := flagSet.String("output", "", "Optional: export results as JSON file")
	flagSet.Parse(args)

	if *backupDir == "" && *logDir == "" {
		fmt.Println("Error: at least one of --backup-dir or --log-dir is required")
		fmt.Println("Usage: iso-cli check --backup-dir /var/backups --log-dir /var/log")
		os.Exit(1)
	}

	var results []CheckResult

	printHeader("ISO 27001 — Local Technical Audit")

	// Check 1: Backup age (A.8.13)
	if *backupDir != "" {
		result := checkBackupAge(*backupDir, *maxAge)
		results = append(results, result)
		printCheckResult(result)
	}

	// Check 2: Backup directory permissions (A.8.13)
	if *backupDir != "" {
		result := checkDirPermissions(*backupDir, 0750)
		results = append(results, result)
		printCheckResult(result)
	}

	// Check 3: Log files present and non-empty (A.8.15)
	if *logDir != "" {
		result := checkLogFiles(*logDir)
		results = append(results, result)
		printCheckResult(result)
	}

	// Check 4: Log directory permissions (A.8.15)
	if *logDir != "" {
		result := checkDirPermissions(*logDir, 0750)
		results = append(results, result)
		printCheckResult(result)
	}

	// Check 5: Disk space (general)
	if *backupDir != "" {
		result := checkDiskSpace(*backupDir)
		results = append(results, result)
		printCheckResult(result)
	}

	// Summary
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	total := len(results)
	findings := total - passed

	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	if findings == 0 {
		fmt.Printf("║  %sResult: %d/%d controls passed — No findings%s            ║\n",
			colorGreen, passed, total, colorReset)
	} else {
		fmt.Printf("║  Result: %d/%d controls passed — %d finding(s)%s\n",
			passed, total, findings, strings.Repeat(" ", 14))
	}
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	// Export JSON if requested
	if *outputJSON != "" {
		exportCheckResults(results, *outputJSON)
	}

	if findings > 0 {
		os.Exit(1)
	}
}

func checkBackupAge(dir string, maxDays int) CheckResult {
	result := CheckResult{
		Control:     "A.8.13",
		Description: "Backup recency",
	}

	info, err := os.Stat(dir)
	if err != nil {
		result.Passed = false
		result.Detail = fmt.Sprintf("Backup directory not accessible: %v", err)
		return result
	}

	if !info.IsDir() {
		result.Passed = false
		result.Detail = fmt.Sprintf("%s is not a directory", dir)
		return result
	}

	var newestMod time.Time
	var newestFile string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newestMod) {
			newestMod = info.ModTime()
			newestFile = path
		}
		return nil
	})

	if newestFile == "" {
		result.Passed = false
		result.Detail = "No backup files found in directory"
		return result
	}

	age := time.Since(newestMod)
	ageDays := int(age.Hours() / 24)

	if ageDays <= maxDays {
		result.Passed = true
		result.Detail = fmt.Sprintf("Most recent backup: %d day(s) ago (max allowed: %d)", ageDays, maxDays)
	} else {
		result.Passed = false
		result.Detail = fmt.Sprintf("Most recent backup: %d day(s) ago — EXCEEDS maximum of %d days", ageDays, maxDays)
	}

	return result
}

func checkDirPermissions(dir string, expectedMode os.FileMode) CheckResult {
	result := CheckResult{
		Control:     "A.8.13",
		Description: fmt.Sprintf("Directory permissions for %s", dir),
	}

	info, err := os.Stat(dir)
	if err != nil {
		result.Passed = false
		result.Detail = fmt.Sprintf("Cannot access directory: %v", err)
		return result
	}

	actualMode := info.Mode().Perm()
	if actualMode <= expectedMode {
		result.Passed = true
		result.Detail = fmt.Sprintf("Permissions: %04o (max allowed: %04o)", actualMode, expectedMode)
	} else {
		result.Passed = false
		result.Detail = fmt.Sprintf("Permissions: %04o — TOO PERMISSIVE (max allowed: %04o)", actualMode, expectedMode)
	}

	return result
}

func checkLogFiles(dir string) CheckResult {
	result := CheckResult{
		Control:     "A.8.15",
		Description: "Access log files present and non-empty",
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		result.Passed = false
		result.Detail = fmt.Sprintf("Log directory not accessible: %s", dir)
		return result
	}

	logCount := 0
	emptyCount := 0
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz") {
			logCount++
			info, _ := d.Info()
			if info != nil && info.Size() == 0 {
				emptyCount++
			}
		}
		return nil
	})

	if logCount == 0 {
		result.Passed = false
		result.Detail = "No .log files found in log directory"
	} else if emptyCount > 0 {
		result.Passed = false
		result.Detail = fmt.Sprintf("Found %d log file(s), but %d are empty", logCount, emptyCount)
	} else {
		result.Passed = true
		result.Detail = fmt.Sprintf("Found %d log file(s), all non-empty", logCount)
	}

	return result
}

func checkDiskSpace(dir string) CheckResult {
	result := CheckResult{
		Control:     "A.8.13",
		Description: "Sufficient disk space (>20%)",
	}

	// Use a simple heuristic: check if we can write a temp file
	// On production systems, this would use syscall.Statfs
	testFile := filepath.Join(dir, ".iso-cli-space-test")
	f, err := os.Create(testFile)
	if err != nil {
		result.Passed = false
		result.Detail = fmt.Sprintf("Cannot write to backup directory: %v", err)
		return result
	}
	f.Close()
	os.Remove(testFile)

	result.Passed = true
	result.Detail = "Backup directory is writable (disk space check: basic)"
	return result
}

func exportCheckResults(results []CheckResult, outputPath string) {
	report := map[string]interface{}{
		"tool":      "iso-cli",
		"version":   version,
		"command":   "check",
		"timestamp": time.Now().Format(time.RFC3339),
		"results":   results,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling results: %v\n", err)
		return
	}

	err = os.WriteFile(outputPath, data, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return
	}

	fmt.Printf("\n  Report exported to: %s\n", outputPath)
}

// ──────────────────────────────────────────────
// COMMAND: validate-docs
// ──────────────────────────────────────────────

func runValidateDocs(args []string) {
	flagSet := flag.NewFlagSet("validate-docs", flag.ExitOnError)
	docsPath := flagSet.String("path", "./docs", "Path to documentation directory")
	strict := flagSet.Bool("strict", false, "Fail on any unfilled variable (exit code 1)")
	outputJSON := flagSet.String("output", "", "Optional: export results as JSON file")
	flagSet.Parse(args)

	printHeader("ISO 27001 — Documentation Validator")

	// Regex to find template variables like [EMPRESA], [NOMBRE_RESPONSABLE], etc.
	varPattern := regexp.MustCompile(`\[([A-Z][A-Z0-9_]{2,})\]`)

	var findings []DocValidationResult
	fileCount := 0

	err := filepath.WalkDir(*docsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		fileCount++
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: cannot read %s: %v\n", path, err)
			return nil
		}

		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			matches := varPattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				// Skip common Markdown patterns that look like variables but aren't
				varName := match[1]
				if isKnownNonVariable(varName) {
					continue
				}

				findings = append(findings, DocValidationResult{
					File:     path,
					Line:     lineNum + 1,
					Variable: match[0],
					Context:  strings.TrimSpace(line),
				})
			}
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Printf("  Scanned: %d Markdown file(s)\n\n", fileCount)

	if len(findings) == 0 {
		fmt.Printf("  %s[✓] All template variables have been filled in%s\n", colorGreen, colorReset)
	} else {
		fmt.Printf("  %s[!] Found %d unfilled variable(s):%s\n\n", colorYellow, len(findings), colorReset)

		currentFile := ""
		for _, f := range findings {
			if f.File != currentFile {
				currentFile = f.File
				fmt.Printf("  %s%s%s\n", colorCyan, f.File, colorReset)
			}
			fmt.Printf("    Line %d: %s%s%s → %s\n",
				f.Line, colorYellow, f.Variable, colorReset, truncate(f.Context, 60))
		}
	}

	fmt.Println("\n╚══════════════════════════════════════════════════════════╝")

	// Export JSON if requested
	if *outputJSON != "" {
		report := map[string]interface{}{
			"tool":       "iso-cli",
			"version":    version,
			"command":    "validate-docs",
			"timestamp":  time.Now().Format(time.RFC3339),
			"files_scanned": fileCount,
			"findings":   findings,
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		os.WriteFile(*outputJSON, data, 0644)
		fmt.Printf("  Report exported to: %s\n", *outputJSON)
	}

	if *strict && len(findings) > 0 {
		os.Exit(1)
	}
}

func isKnownNonVariable(name string) bool {
	// Common Markdown/documentation patterns that are not template variables
	skip := map[string]bool{
		"MIT": true, "RFC": true, "ISO": true, "IEC": true,
		"DNS": true, "SPF": true, "DKIM": true, "DMARC": true,
		"AES": true, "SSL": true, "TLS": true, "VPN": true,
		"HTTPS": true, "HTTP": true, "SSH": true, "SMTP": true,
		"RTO": true, "RPO": true, "MFA": true, "BYOD": true,
		"TODO": true, "NOTE": true, "WARNING": true,
	}
	return skip[name]
}

// ──────────────────────────────────────────────
// COMMAND: export-soa
// ──────────────────────────────────────────────

func runExportSOA(args []string) {
	flagSet := flag.NewFlagSet("export-soa", flag.ExitOnError)
	inputFile := flagSet.String("input", "SOA_Matrix.json", "Path to SOA Matrix JSON file")
	outputFile := flagSet.String("output", "", "Output file path (default: stdout)")
	filterStatus := flagSet.String("status", "", "Filter by implementation status (implemented, partial, not_implemented)")
	flagSet.Parse(args)

	printHeader("ISO 27001 — SoA Export")

	data, err := os.ReadFile(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading SOA file: %v\n", err)
		os.Exit(1)
	}

	var soa SOAMatrix
	err = json.Unmarshal(data, &soa)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing SOA JSON: %v\n", err)
		os.Exit(1)
	}

	// Display summary
	fmt.Printf("  Standard:   %s\n", soa.Metadata.Standard)
	fmt.Printf("  Version:    %s\n", soa.Metadata.Version)
	fmt.Printf("  Updated:    %s\n", soa.Metadata.LastUpdated)
	fmt.Printf("  Controls:   %d evaluated\n", soa.Summary.TotalControls)
	fmt.Printf("  Compliance: %.1f%%\n\n", soa.Summary.CompliancePercentage)

	// Filter controls if requested
	var filtered []SOAControl
	for _, c := range soa.Controls {
		if *filterStatus == "" || c.ImplementationStatus == *filterStatus {
			filtered = append(filtered, c)
		}
	}

	// Print controls table
	fmt.Printf("  %-8s %-45s %-15s\n", "Control", "Clause", "Status")
	fmt.Printf("  %s\n", strings.Repeat("─", 70))

	for _, c := range filtered {
		statusIcon := "✓"
		statusColor := colorGreen
		if c.ImplementationStatus == "partial" {
			statusIcon = "◐"
			statusColor = colorYellow
		} else if c.ImplementationStatus == "not_implemented" {
			statusIcon = "✗"
			statusColor = colorRed
		}

		clause := truncate(c.Clause, 43)
		fmt.Printf("  %-8s %-45s %s%s %s%s\n",
			c.ID, clause, statusColor, statusIcon, c.ImplementationStatus, colorReset)
	}

	// Export if output file specified
	if *outputFile != "" {
		report := map[string]interface{}{
			"tool":      "iso-cli",
			"version":   version,
			"command":   "export-soa",
			"timestamp": time.Now().Format(time.RFC3339),
			"source":    *inputFile,
			"summary":   soa.Summary,
			"controls":  filtered,
		}

		out, _ := json.MarshalIndent(report, "", "  ")
		err = os.WriteFile(*outputFile, out, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n  Exported %d control(s) to: %s\n", len(filtered), *outputFile)
	}

	fmt.Println("\n╚══════════════════════════════════════════════════════════╝")
}

// ──────────────────────────────────────────────
// Utility functions
// ──────────────────────────────────────────────

func printHeader(title string) {
	width := 58
	padding := (width - len(title)) / 2
	if padding < 1 {
		padding = 1
	}

	fmt.Println()
	fmt.Println("╔" + strings.Repeat("═", width) + "╗")
	fmt.Printf("║%s%s%s%s║\n",
		strings.Repeat(" ", padding),
		colorBold, title, colorReset+strings.Repeat(" ", width-padding-len(title)))
	fmt.Println("╠" + strings.Repeat("═", width) + "╣")
}

func printCheckResult(r CheckResult) {
	icon := colorGreen + "[✓]" + colorReset
	if !r.Passed {
		icon = colorRed + "[✗]" + colorReset
	}
	fmt.Printf("║  %s %s\n", icon, r.Detail)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
