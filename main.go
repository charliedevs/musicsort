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

	// Resolve absolute paths once to avoid repeated syscalls in workers
	absSrcDir, err := filepath.Abs(sourceDir)
	if err != nil {
		log.Fatal(err)
	}
	absTgtDir, err := filepath.Abs(targetDir)
	if err != nil {
		log.Fatal(err)
	}

	if dryRun {
		fmt.Println("\033[1;33m[DRY RUN] No files will actually be moved.\033[0m")
	}

	var wg sync.WaitGroup
	filesToProcess := make(chan string, 100)

	// Start worker pool (20 goroutines)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range filesToProcess {
				processFile(path, absTgtDir)
			}
		}()
	}

	// Walk source directory to find music files
	err = filepath.WalkDir(absSrcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !recursive && path != absSrcDir {
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

	removeEmptyDirs(absSrcDir)

	fmt.Println("Done!")
}

func sanitize(s string) string {
	return illegalChars.ReplaceAllString(s, "_")
}

func processFile(path, absTgtDir string) {
	artist, album, title, year, err := readTags(path)
	if err != nil {
		log.Printf("Error reading tags for %s: %v", path, err)
		return
	}

	sArtist := sanitize(artist)
	sAlbum := sanitize(album) + year
	sTitle := sanitize(title)
	ext := filepath.Ext(path)

	// Construct absolute destination path
	destDir := filepath.Join(absTgtDir, sArtist, sAlbum)
	newFilename := fmt.Sprintf("%s - %s%s", sArtist, sTitle, ext)
	destPath := filepath.Join(destDir, newFilename)

	// Ignore unchanged files
	if path == destPath {
		return
	}

	if dryRun {
		fmt.Printf("[DRY-RUN] %s -> %s\n", path, destPath)
		return
	}

	// Ensure destination exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		log.Printf("Error creating directory %s: %v", destDir, err)
		return
	}

	// Check if file already exists at destination
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("\033[0;33m[SKIP]\033[0m %s already exists\n", newFilename)
		return
	}

	fmt.Printf("\033[0;32m[MOVE]\033[0m %s\n", newFilename)
	if err := moveFile(path, destPath); err != nil {
		log.Printf("Error moving %s: %v", path, err)
	}
}

// readTags handles file opening and metadata extraction
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

	// Default fallbacks for missing tags
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

// moveFile supports cross-device moves by falling back to copy+delete
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Standard fallback for "invalid cross-device link" errors
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

	// Sort by path length descending (bottom-up cleanup)
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	for _, dir := range dirs {
		if dryRun {
			// Check if directory is empty to report accurately in dry run
			entries, err := os.ReadDir(dir)
			if err == nil && len(entries) == 0 {
				fmt.Printf("[DRY-RUN] Would remove empty folder: %s\n", dir)
			}
			continue
		}

		// os.Remove only succeeds if the directory is empty
		if err := os.Remove(dir); err == nil {
			fmt.Printf("\033[0;31m[REMOVE]\033[0m %s\n", dir)
		}
	}
}
