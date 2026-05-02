package musicsort

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var supportedExtensions = map[string]bool{
	".mp3":  true,
	".m4a":  true,
	".flac": true,
	".ogg":  true,
	".opus": true,
	".wav":  true,
	".aac":  true,
}

// TrackOrganizer holds state for organizing music files.
type TrackOrganizer struct {
	albumMap  map[string]string
	mapMutex  sync.Mutex
	targetDir string
	dryRun    bool
}

// Result holds statistics about the organization operation.
type Result struct {
	Total   int
	Moved   int
	Skipped int
	Errors  int
}

// Run organizes music files from source to target directory.
func Run(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if cfg.DryRun {
		PrintDryRunWarning()
	}

	org := &TrackOrganizer{
		albumMap:  make(map[string]string),
		targetDir: cfg.TargetDir,
		dryRun:    cfg.DryRun,
	}

	// Pre-scan target directory to populate albumMap with existing structure
	if err := org.scanExistingStructure(); err != nil {
		return fmt.Errorf("scan target directory: %w", err)
	}

	var wg sync.WaitGroup
	filesToProcess := make(chan string, 100)

	// Start worker pool (20 goroutines)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range filesToProcess {
				org.processFile(path)
			}
		}()
	}

	// Walk source directory to find music files
	err := filepath.WalkDir(cfg.SourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !cfg.Recursive && path != cfg.SourceDir {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if supportedExtensions[ext] {
			filesToProcess <- path
		}
		return nil
	})

	if err != nil {
		log.Printf("error walking directory: %v", err)
	}

	close(filesToProcess)
	wg.Wait()

	RemoveEmptyDirs(cfg.SourceDir, cfg.DryRun)
	PrintDone()

	return nil
}

// scanExistingStructure builds the initial map of Album -> Artist from the destination.
func (o *TrackOrganizer) scanExistingStructure() error {
	return filepath.WalkDir(o.targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		// Expecting structure: target/Artist/Album
		rel, _ := filepath.Rel(o.targetDir, path)
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) == 2 {
			o.albumMap[parts[1]] = parts[0]
		}
		return nil
	})
}

// processFile processes a single audio file for organization.
func (o *TrackOrganizer) processFile(path string) {
	meta, err := ReadFileMetadata(path)
	if err != nil {
		log.Printf("error reading tags for %s: %v", path, err)
		return
	}

	sArtist := Sanitize(meta.Artist)
	sAlbum := Sanitize(meta.Album) + meta.Year
	sTitle := Sanitize(meta.Title)
	ext := filepath.Ext(path)

	// Thread-safe album-to-artist resolution
	o.mapMutex.Lock()
	if existingArtist, ok := o.albumMap[sAlbum]; ok {
		lowerNew := strings.ToLower(sArtist)
		lowerOld := strings.ToLower(existingArtist)

		// Check if one artist name is contained within the other (Features)
		if strings.Contains(lowerNew, lowerOld) || strings.Contains(lowerOld, lowerNew) {
			if len(sArtist) < len(existingArtist) {
				// New name is shorter and matches, upgrade the directory
				oldPath := filepath.Join(o.targetDir, existingArtist, sAlbum)
				newPath := filepath.Join(o.targetDir, sArtist, sAlbum)
				if !o.dryRun {
					os.MkdirAll(filepath.Join(o.targetDir, sArtist), 0755)
					os.Rename(oldPath, newPath)
				}
				o.albumMap[sAlbum] = sArtist
			} else {
				// Existing name is shorter or equal, stick with it
				sArtist = existingArtist
			}
		}
	} else {
		o.albumMap[sAlbum] = sArtist
	}
	o.mapMutex.Unlock()

	// Construct absolute destination path
	destDir := filepath.Join(o.targetDir, sArtist, sAlbum)
	newFilename := fmt.Sprintf("%s - %s%s", sArtist, sTitle, ext)
	destPath := filepath.Join(destDir, newFilename)

	// Ignore unchanged files
	if path == destPath {
		return
	}

	if o.dryRun {
		fmt.Printf("[DRY-RUN] %s -> %s\n", path, destPath)
		return
	}

	// Ensure destination exists
	if err := CreateDirectoryPath(destDir); err != nil {
		log.Printf("error creating directory %s: %v", destDir, err)
		return
	}

	// Check if file already exists at destination
	if FileExists(destPath) {
		PrintSkip(newFilename, "already exists")
		return
	}

	PrintMove(newFilename)
	if err := MoveFile(path, destPath, o.dryRun); err != nil {
		log.Printf("error moving %s: %v", path, err)
	}
}
