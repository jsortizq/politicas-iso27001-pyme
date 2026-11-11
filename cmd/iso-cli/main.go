// iso-cli — ISO 27001:2022 Compliance Automation Tool
//
// A CLI tool for SMEs to automate technical audits against ISO 27001 controls.
//
// Subcommands:
//
//	check          Run a local technical audit (backups, logs, permissions, disk)
//	validate-docs  Verify that Markdown docs have no unfilled template variables
//	export-soa     Export compliance status from SOA_Matrix.json
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/checker"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/report"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/soa"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/validator"
)

// ANSI color codes for terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

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
		fmt.Printf("iso-cli v%s — ISO 27001:2022 Compliance Tool\n", report.Version())
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
	minFree := flagSet.Float64("min-free-pct", 20, "Minimum free disk space percentage")
	outputJSON := flagSet.String("output", "", "Optional: export results as JSON file")
	flagSet.Parse(args)

	if *backupDir == "" && *logDir == "" {
		fmt.Println("Error: at least one of --backup-dir or --log-dir is required")
		fmt.Println("Usage: iso-cli check --backup-dir /var/backups --log-dir /var/log")
		os.Exit(1)
	}

	var results []checker.Result

	printHeader("ISO 27001 — Local Technical Audit")

	// Check 1: Backup age (A.8.13)
	if *backupDir != "" {
		result := checker.BackupAge(*backupDir, *maxAge)
		results = append(results, result)
		printCheckResult(result)
	}

	// Check 2: Backup directory permissions (A.8.13)
	if *backupDir != "" {
		result := checker.DirPermissions(*backupDir, 0750, "A.8.13")
		results = append(results, result)
		printCheckResult(result)
	}

	// Check 3: Log files present and non-empty (A.8.15)
	if *logDir != "" {
		result := checker.LogFiles(*logDir)
		results = append(results, result)
		printCheckResult(result)
	}

	// Check 4: Log directory permissions (A.8.15)
	if *logDir != "" {
		result := checker.DirPermissions(*logDir, 0750, "A.8.15")
		results = append(results, result)
		printCheckResult(result)
	}

	// Check 5: Disk space (A.8.13)
	if *backupDir != "" {
		result := checker.DiskSpace(*backupDir, *minFree)
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
		data := map[string]interface{}{"results": results}
		if err := report.Export(*outputJSON, "check", data); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		} else {
			fmt.Printf("\n  Report exported to: %s\n", *outputJSON)
		}
	}

	if findings > 0 {
		os.Exit(1)
	}
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

	findings, fileCount, err := validator.ValidateDocs(*docsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

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
			ctx := f.Context
			if len(ctx) > 60 {
				ctx = ctx[:57] + "..."
			}
			fmt.Printf("    Line %d: %s%s%s → %s\n",
				f.Line, colorYellow, f.Variable, colorReset, ctx)
		}
	}

	fmt.Println("\n╚══════════════════════════════════════════════════════════╝")

	if *outputJSON != "" {
		data := map[string]interface{}{
			"files_scanned": fileCount,
			"findings":      findings,
		}
		if err := report.Export(*outputJSON, "validate-docs", data); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		} else {
			fmt.Printf("  Report exported to: %s\n", *outputJSON)
		}
	}

	if *strict && len(findings) > 0 {
		os.Exit(1)
	}
}

// ──────────────────────────────────────────────
// COMMAND: export-soa
// ──────────────────────────────────────────────

func runExportSOA(args []string) {
	flagSet := flag.NewFlagSet("export-soa", flag.ExitOnError)
	inputFile := flagSet.String("input", "SOA_Matrix.json", "Path to SOA Matrix JSON file")
	outputFile := flagSet.String("output", "", "Output file path (default: stdout)")
	filterStatus := flagSet.String("status", "", "Filter by status (implemented, partial, not_implemented)")
	flagSet.Parse(args)

	printHeader("ISO 27001 — SoA Export")

	matrix, err := soa.Load(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading SOA: %v\n", err)
		os.Exit(1)
	}

	// Recompute summary from actual controls (fixes inconsistencies)
	computed := soa.ComputeSummary(matrix.Controls)

	fmt.Printf("  Standard:   %s\n", matrix.Metadata.Standard)
	fmt.Printf("  Version:    %s\n", matrix.Metadata.Version)
	fmt.Printf("  Updated:    %s\n", matrix.Metadata.LastUpdated)
	fmt.Printf("  Controls:   %d evaluated\n", computed.TotalControls)
	fmt.Printf("  Compliance: %.1f%%\n\n", computed.CompliancePercentage)

	// Filter controls if requested
	filtered := soa.FilterByStatus(matrix.Controls, *filterStatus)

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

		clause := c.Clause
		if len(clause) > 43 {
			clause = clause[:40] + "..."
		}
		fmt.Printf("  %-8s %-45s %s%s %s%s\n",
			c.ID, clause, statusColor, statusIcon, c.ImplementationStatus, colorReset)
	}

	// Export if output file specified
	if *outputFile != "" {
		data := map[string]interface{}{
			"source":   *inputFile,
			"summary":  computed,
			"controls": filtered,
		}
		if err := report.Export(*outputFile, "export-soa", data); err != nil {
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

func printCheckResult(r checker.Result) {
	icon := colorGreen + "[✓]" + colorReset
	if !r.Passed {
		icon = colorRed + "[✗]" + colorReset
	}
	fmt.Printf("║  %s %s\n", icon, r.Detail)
}
