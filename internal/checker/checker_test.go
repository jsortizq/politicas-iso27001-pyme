package checker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupAge_RecentBackup(t *testing.T) {
	dir := t.TempDir()
	// Create a file modified just now
	f, err := os.Create(filepath.Join(dir, "backup-2024.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	result := BackupAge(dir, 7)
	if !result.Passed {
		t.Errorf("expected pass for recent backup, got fail: %s", result.Detail)
	}
	if result.Control != "A.8.13" {
		t.Errorf("expected control A.8.13, got %s", result.Control)
	}
}

func TestBackupAge_OldBackup(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "backup-old.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Set modification time to 30 days ago
	old := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "backup-old.tar.gz"), old, old)

	result := BackupAge(dir, 7)
	if result.Passed {
		t.Errorf("expected fail for old backup, got pass: %s", result.Detail)
	}
}

func TestBackupAge_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := BackupAge(dir, 7)
	if result.Passed {
		t.Error("expected fail for empty directory")
	}
}

func TestBackupAge_NonexistentDir(t *testing.T) {
	result := BackupAge("/nonexistent/path/backup", 7)
	if result.Passed {
		t.Error("expected fail for nonexistent directory")
	}
}

func TestDirPermissions_Acceptable(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0750)

	result := DirPermissions(dir, 0750, "A.8.13")
	if !result.Passed {
		t.Errorf("expected pass for 0750 permissions: %s", result.Detail)
	}
	if result.Control != "A.8.13" {
		t.Errorf("expected control A.8.13, got %s", result.Control)
	}
}

func TestDirPermissions_TooPermissive(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0777)

	result := DirPermissions(dir, 0750, "A.8.15")
	if result.Passed {
		t.Errorf("expected fail for 0777 permissions: %s", result.Detail)
	}
	if result.Control != "A.8.15" {
		t.Errorf("expected control A.8.15, got %s", result.Control)
	}
}

func TestDirPermissions_ControlParameterized(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)

	r1 := DirPermissions(dir, 0750, "A.8.13")
	r2 := DirPermissions(dir, 0750, "A.8.15")

	if r1.Control == r2.Control {
		t.Error("controls should differ when different values passed")
	}
}

func TestLogFiles_Present(t *testing.T) {
	dir := t.TempDir()
	// Create a non-empty log file
	os.WriteFile(filepath.Join(dir, "access.log"), []byte("log entry\n"), 0644)

	result := LogFiles(dir)
	if !result.Passed {
		t.Errorf("expected pass for non-empty log: %s", result.Detail)
	}
	if result.Control != "A.8.15" {
		t.Errorf("expected control A.8.15, got %s", result.Control)
	}
}

func TestLogFiles_Empty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "access.log"), []byte{}, 0644)

	result := LogFiles(dir)
	if result.Passed {
		t.Error("expected fail for empty log file")
	}
}

func TestLogFiles_NoLogs(t *testing.T) {
	dir := t.TempDir()
	result := LogFiles(dir)
	if result.Passed {
		t.Error("expected fail when no log files present")
	}
}

func TestDiskSpace_Sufficient(t *testing.T) {
	// Use temp dir which should have space
	dir := t.TempDir()
	result := DiskSpace(dir, 1) // 1% — should pass on any real system
	if !result.Passed {
		t.Errorf("expected pass for 1%% threshold: %s", result.Detail)
	}
	if result.Control != "A.8.13" {
		t.Errorf("expected control A.8.13, got %s", result.Control)
	}
}

func TestDiskSpace_NonexistentDir(t *testing.T) {
	result := DiskSpace("/nonexistent/path", 20)
	if result.Passed {
		t.Error("expected fail for nonexistent directory")
	}
}
