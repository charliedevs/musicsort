package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/dhowden/tag"
)

var (
	sourceDir  string
	targetDir  string
	recursive  bool
	dryRun     bool
	extensions = map[string]bool{
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
	filesToProcess := make(chan string)

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

	close(filesToProcess)
	wg.Wait()

	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Done!")
}

func sanitize(s string) string {
	reg := regexp.MustCompile(`[/\\?%*:|"<>]+`)
	return reg.ReplaceAllString(s, "_")
}

func processFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	// Corrected API: ReadFrom
	m, err := tag.ReadFrom(f)
	var artist, album, title, year string

	if err == nil {
		artist = m.Artist()
		album = m.Album()
		title = m.Title()
		
		if y := m.Year(); y > 0 {
			year = fmt.Sprintf(" (%d)", y)
		}
	}

	// Fallbacks
	if artist == "" { artist = "Unknown Artist" }
	if album == "" { album = "Unknown Album" }
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
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
		return
	}

	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("\033[0;33m[SKIP]\033[0m %s already exists\n", newFilename)
	} else {
		fmt.Printf("\033[0;32m[MOVE]\033[0m %s\n", newFilename)
		os.Rename(path, destPath)
	}
}
