// Package checker implements technical audit checks against ISO 27001 controls.
//
// Each check evaluates a specific technical condition (backup recency,
// directory permissions, log integrity, disk space) and returns a
// structured result that can be rendered or exported.
package checker

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Result represents the outcome of a single audit check.
type Result struct {
	Control     string `json:"control"`
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Detail      string `json:"detail"`
}

// BackupAge checks whether the most recent file in dir was modified within
// maxDays. Maps to ISO 27001 control A.8.13 (Information backup).
func BackupAge(dir string, maxDays int) Result {
	result := Result{
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
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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

// DirPermissions checks that a directory's permissions do not exceed
// expectedMode. The control parameter specifies which ISO control this
// check maps to (A.8.13 for backups, A.8.15 for logs).
func DirPermissions(dir string, expectedMode os.FileMode, control string) Result {
	result := Result{
		Control:     control,
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

// LogFiles checks that a directory contains non-empty .log or .log.gz files.
// Maps to ISO 27001 control A.8.15 (Logging).
func LogFiles(dir string) Result {
	result := Result{
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
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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

// DiskSpace checks actual free disk space on the filesystem containing dir.
// Requires at least minFreePercent (e.g., 20 for 20%) free space.
// Maps to ISO 27001 control A.8.13 (Information backup).
func DiskSpace(dir string, minFreePercent float64) Result {
	result := Result{
		Control:     "A.8.13",
		Description: fmt.Sprintf("Sufficient disk space (>%.0f%%)", minFreePercent),
	}

	var stat syscall.Statfs_t
	err := syscall.Statfs(dir, &stat)
	if err != nil {
		result.Passed = false
		result.Detail = fmt.Sprintf("Cannot check disk space: %v", err)
		return result
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)

	if totalBytes == 0 {
		result.Passed = false
		result.Detail = "Filesystem reports zero total size"
		return result
	}

	freePercent := float64(freeBytes) / float64(totalBytes) * 100
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)

	if freePercent >= minFreePercent {
		result.Passed = true
		result.Detail = fmt.Sprintf("Free space: %.1f%% (%.1f GB available)", freePercent, freeGB)
	} else {
		result.Passed = false
		result.Detail = fmt.Sprintf("Free space: %.1f%% (%.1f GB) — BELOW minimum of %.0f%%", freePercent, freeGB, minFreePercent)
	}

	return result
}
