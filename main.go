package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/dhowden/tag"
)

var (
	sourceDir    string
	targetDir    string
	recursive    bool
	dryRun       bool
	illegalChars = regexp.MustCompile(`[/\\?%*:|"<>]+`)
	extensions   = map[string]bool{
		".mp3": true, ".m4a": true, ".flac": true,
		".ogg": true, ".opus": true, ".wav": true, ".aac": true,
	}
)

func main() {
	flag.StringVar(&sourceDir, "s", ".", "Source directory")
	flag.StringVar(&targetDir, "t", ".", "Target directory")
	flag.BoolVar(&recursive, "r", false, "Enable recursive search")
	flag.BoolVar(&dryRun, "n", false, "Dry run")
	flag.Parse()

	if dryRun {
		fmt.Println("\033[1;33m[DRY RUN] No files will actually be moved.\033[0m")
	}

	var wg sync.WaitGroup
	filesToProcess := make(chan string, 100)

	// Start workers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range filesToProcess {
				processFile(path)
			}
		}()
	}

	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !recursive && path != sourceDir {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if extensions[ext] {
			filesToProcess <- path
		}
		return nil
	})

	if err != nil {
		log.Printf("Error walking directory: %v", err)
	}

	close(filesToProcess)
	wg.Wait()

	fmt.Println("Cleaning up any empty source directories...")
	removeEmptyDirs(sourceDir)

	fmt.Println("Done!")
}

func sanitize(s string) string {
	return illegalChars.ReplaceAllString(s, "_")
}

func processFile(path string) {
	artist, album, title, year, err := readTags(path)
	if err != nil {
		// Log error but continue to next file
		log.Printf("Error reading tags for %s: %v", path, err)
		return
	}

	sArtist := sanitize(artist)
	sAlbum := sanitize(album) + year
	sTitle := sanitize(title)
	ext := filepath.Ext(path)

	destDir := filepath.Join(targetDir, sArtist, sAlbum)
	newFilename := fmt.Sprintf("%s - %s%s", sArtist, sTitle, ext)
	destPath := filepath.Join(destDir, newFilename)

	if dryRun {
		fmt.Printf("[DRY-RUN] %s -> %s\n", path, destPath)
		return
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		log.Printf("Error creating directory %s: %v", destDir, err)
		return
	}

	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("\033[0;33m[SKIP]\033[0m %s already exists\n", newFilename)
		return
	}

	fmt.Printf("\033[0;32m[MOVE]\033[0m %s\n", newFilename)
	if err := moveFile(path, destPath); err != nil {
		log.Printf("Error moving %s: %v", path, err)
	}
}

// readTags extracts metadata and closes the file immediately
func readTags(path string) (artist, album, title, year string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", "", "", err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err == nil {
		artist = m.Artist()
		album = m.Album()
		title = m.Title()
		if y := m.Year(); y > 0 {
			year = fmt.Sprintf(" (%d)", y)
		}
	}

	// Fallbacks
	if artist == "" {
		artist = "Unknown Artist"
	}
	if album == "" {
		album = "Unknown Album"
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	return artist, album, title, year, nil
}

// moveFile attempts a rename, but falls back to copy/delete for cross-device moves
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// If rename failed, try manual copy (standard for cross-partition moves)
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Close files before removing source
	sourceFile.Close()
	return os.Remove(src)
}

func removeEmptyDirs(root string) {
	var dirs []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})

	// Sort deepest first
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	for _, dir := range dirs {
		if dryRun {
			entries, _ := os.ReadDir(dir)
			if len(entries) == 0 {
				fmt.Printf("[DRY-RUN] Would remove empty folder: %s\n", dir)
			}
			continue
		}
		// Silently ignore errors (dir might not be empty)
		_ = os.Remove(dir)
	}
}
