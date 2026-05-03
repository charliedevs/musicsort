package musicsort

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "Hello World"},
		{"Artist: The Best", "Artist_ The Best"},
		{"Album/2024", "Album_2024"},
		{"Track|Name*Illegal\"Chars", "Track_Name_Illegal_Chars"},
		{"Normal-File (2024)", "Normal-File (2024)"},
	}

	for _, tt := range tests {
		got := Sanitize(tt.input)
		if got != tt.want {
			t.Errorf("Sanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := Config{
		SourceDir: ".",
		TargetDir: ".",
		Recursive: false,
		DryRun:    false,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}

	if cfg.SourceDir == "." || cfg.TargetDir == "." {
		t.Fatalf("Validate() did not resolve relative paths")
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	if FileExists(path) {
		t.Fatalf("FileExists(%q) returned true for non-existent file", path)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if !FileExists(path) {
		t.Fatalf("FileExists(%q) returned false for existing file", path)
	}
}

func TestCreateDirectoryPath(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "a", "b", "c")

	if err := CreateDirectoryPath(path); err != nil {
		t.Fatalf("CreateDirectoryPath() failed: %v", err)
	}

	if !FileExists(path) {
		t.Fatalf("CreateDirectoryPath() did not create directory %q", path)
	}

	// Should be idempotent
	if err := CreateDirectoryPath(path); err != nil {
		t.Fatalf("CreateDirectoryPath() failed on second call: %v", err)
	}
}

func TestMoveFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	// Create source file
	if err := os.WriteFile(src, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Move file
	if err := MoveFile(src, dst, false); err != nil {
		t.Fatalf("MoveFile() failed: %v", err)
	}

	if FileExists(src) {
		t.Fatalf("Source file still exists after move")
	}

	if !FileExists(dst) {
		t.Fatalf("Destination file was not created")
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "test content" {
		t.Fatalf("File content was corrupted")
	}
}

func TestMoveFileDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	// Create source file
	if err := os.WriteFile(src, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Move file in dry-run mode
	if err := MoveFile(src, dst, true); err != nil {
		t.Fatalf("MoveFile() in dry-run failed: %v", err)
	}

	// Source should still exist in dry-run mode
	if !FileExists(src) {
		t.Fatalf("Source file was removed in dry-run mode")
	}

	if FileExists(dst) {
		t.Fatalf("Destination file was created in dry-run mode")
	}
}

func TestRemoveEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	emptyDir := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}
	filledDir := filepath.Join(tmpDir, "c")
	if err := os.MkdirAll(filledDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filledDir, "file.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	RemoveEmptyDirs(tmpDir, false, false)
	if FileExists(emptyDir) {
		t.Fatalf("expected empty directory %q to be removed", emptyDir)
	}
	if !FileExists(filledDir) {
		t.Fatalf("expected non-empty directory %q to remain", filledDir)
	}

	// Dry-run should leave directories intact
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}
	RemoveEmptyDirs(tmpDir, true, false)
	if !FileExists(emptyDir) {
		t.Fatalf("expected empty directory %q to remain in dry-run mode", emptyDir)
	}
}
