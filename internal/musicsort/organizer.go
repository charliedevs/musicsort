package musicsort

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"musicsort/internal/audio"
	"musicsort/internal/clioutput"
)

// TrackOrganizer holds state for organizing music files.
type TrackOrganizer struct {
	albumMap    map[string][]string
	mapMutex    sync.Mutex
	result      Result
	resultMutex sync.Mutex
	processed   int
	targetDir   string
	dryRun      bool
	verbose     bool
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

	org := &TrackOrganizer{
		albumMap:  make(map[string][]string),
		result:    Result{},
		targetDir: cfg.TargetDir,
		dryRun:    cfg.DryRun,
		verbose:   cfg.Verbose,
	}

	if cfg.DryRun {
		PrintDryRunWarning()
	}

	// Pre-scan target directory to populate albumMap with existing structure
	if err := org.scanExistingStructure(); err != nil {
		return fmt.Errorf("scan target directory: %w", err)
	}

	var filePaths []string
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
		if audio.SupportedExtensions[ext] {
			filePaths = append(filePaths, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("walk source directory: %w", err)
	}

	org.result.Total = len(filePaths)
	if org.result.Total == 0 {
		clioutput.InfoLine("No audio files found.")
		return nil
	}

	if cfg.Verbose {
		clioutput.InfoLine("Found %d audio files", org.result.Total)
	} else {
		clioutput.ProgressLine("Processing %d/%d files...", 0, org.result.Total)
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
				processed := org.incrementProcessed()
				if !cfg.Verbose {
					clioutput.ProgressLine("Processing %d/%d files...", processed, org.result.Total)
				}
			}
		}()
	}

	for _, path := range filePaths {
		filesToProcess <- path
	}
	close(filesToProcess)
	wg.Wait()

	if !cfg.Verbose {
		fmt.Println()
	}

	RemoveEmptyDirs(cfg.SourceDir, cfg.DryRun, cfg.Verbose)
	PrintSummary(org.result, cfg.DryRun)

	return nil
}

func (o *TrackOrganizer) incrementProcessed() int {
	o.resultMutex.Lock()
	defer o.resultMutex.Unlock()
	o.processed++
	return o.processed
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
			album := parts[1]
			artist := parts[0]
			o.albumMap[album] = append(o.albumMap[album], artist)
		}
		return nil
	})
}

// processFile processes a single audio file for organization.
func (o *TrackOrganizer) processFile(path string) {
	meta, err := ReadFileMetadata(path)
	if err != nil {
		log.Printf("error reading tags for %s: %v", path, err)
		o.addResult(0, 0, 1)
		return
	}

	sArtist := Sanitize(meta.Artist)
	sAlbum := Sanitize(meta.Album) + meta.Year
	sTitle := Sanitize(meta.Title)
	ext := filepath.Ext(path)

	// Thread-safe album-to-artist resolution
	o.mapMutex.Lock()
	if existingArtists, ok := o.albumMap[sAlbum]; ok {
		bestArtist := ""
		for i, existingArtist := range existingArtists {
			lowerNew := strings.ToLower(sArtist)
			lowerOld := strings.ToLower(existingArtist)
			if strings.Contains(lowerNew, lowerOld) || strings.Contains(lowerOld, lowerNew) {
				if len(sArtist) < len(existingArtist) {
					bestArtist = sArtist
					oldPath := filepath.Join(o.targetDir, existingArtist, sAlbum)
					newPath := filepath.Join(o.targetDir, sArtist, sAlbum)
					if !o.dryRun {
						os.MkdirAll(filepath.Join(o.targetDir, sArtist), 0755)
						os.Rename(oldPath, newPath)
					}
					existingArtists[i] = sArtist
				} else {
					bestArtist = existingArtist
				}
				break
			}
		}
		if bestArtist != "" {
			sArtist = bestArtist
			for _, artist := range existingArtists {
				if artist == sArtist {
					o.albumMap[sAlbum] = existingArtists
					break
				}
			}
		} else {
			o.albumMap[sAlbum] = append(existingArtists, sArtist)
		}
	} else {
		o.albumMap[sAlbum] = []string{sArtist}
	}
	o.mapMutex.Unlock()

	// Construct absolute destination path
	destDir := filepath.Join(o.targetDir, sArtist, sAlbum)
	newFilename := fmt.Sprintf("%s - %s%s", sArtist, sTitle, ext)
	destPath := filepath.Join(destDir, newFilename)

	// Ignore unchanged files
	if path == destPath {
		o.addResult(0, 1, 0)
		return
	}

	// Check if file already exists at destination
	if FileExists(destPath) {
		if o.verbose {
			PrintSkip(newFilename, "already exists")
		}
		o.addResult(0, 1, 0)
		return
	}

	if o.dryRun {
		if o.verbose {
			clioutput.InfoLine("%s %s -> %s", clioutput.Label("DRY-RUN", clioutput.Yellow), path, destPath)
		}
		o.addResult(1, 0, 0)
		return
	}

	// Ensure destination exists
	if err := CreateDirectoryPath(destDir); err != nil {
		log.Printf("error creating directory %s: %v", destDir, err)
		o.addResult(0, 0, 1)
		return
	}

	// Check if file already exists at destination
	if FileExists(destPath) {
		if o.verbose {
			PrintSkip(newFilename, "already exists")
		}
		o.addResult(0, 1, 0)
		return
	}

	if o.verbose {
		PrintMove(newFilename)
	}
	if err := MoveFile(path, destPath, o.dryRun); err != nil {
		log.Printf("error moving %s: %v", path, err)
		o.addResult(0, 0, 1)
		return
	}
	o.addResult(1, 0, 0)
}

func (o *TrackOrganizer) addResult(moved, skipped, errors int) {
	o.resultMutex.Lock()
	defer o.resultMutex.Unlock()
	o.result.Moved += moved
	o.result.Skipped += skipped
	o.result.Errors += errors
}
