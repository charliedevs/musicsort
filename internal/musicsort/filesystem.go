package musicsort

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// FileExists checks if a file or directory exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CreateDirectoryPath creates a directory and all parent directories.
// It is idempotent and does not error if the directory already exists.
func CreateDirectoryPath(path string) error {
	return os.MkdirAll(path, 0755)
}

// MoveFile moves a file from src to dst, falling back to copy+delete for cross-device moves.
func MoveFile(src, dst string, dryRun bool) error {
	if dryRun {
		return nil
	}

	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Standard fallback for "invalid cross-device link" errors
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcFile.Close()
	return os.Remove(src)
}

// RemoveEmptyDirs removes all empty directories under root, walking bottom-up.
// It silently ignores non-empty directories and errors.
func RemoveEmptyDirs(root string, dryRun, verbose bool) {
	var dirs []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})

	// Sort by path length descending (bottom-up cleanup)
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		if len(entries) != 0 {
			continue
		}

		if dryRun {
			if verbose {
				fmt.Printf("[DRY-RUN] Would remove empty folder: %s\n", dir)
			}
			continue
		}

		if err := os.Remove(dir); err == nil {
			if verbose {
				fmt.Printf("\033[0;31m[REMOVE]\033[0m %s\n", dir)
			}
		}
	}
}
