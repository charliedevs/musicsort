package musicsort

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"musicsort/internal/audio"
	"musicsort/internal/clioutput"
	"musicsort/internal/musicmatch"
)

// TrackOrganizer holds state for organizing music files.
type TrackOrganizer struct {
	index       *TargetIndex
	indexMutex  sync.Mutex
	layout      *Layout
	filenameOpt FilenameOpts
	result      Result
	resultMutex sync.Mutex
	processed   int
	targetDir   string
	dryRun      bool
	verbose     bool
}

// Result holds statistics about the organization operation.
type Result struct {
	Total          int
	Moved          int
	Skipped        int
	Errors         int
	Consolidated   int // pre-existing dupe folders merged into a canonical one
	RenamedOnMerge int // collisions during merge that were resolved by renaming
}

// Run organizes music files from source to target directory.
func Run(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	index, err := BuildTargetIndex(cfg.TargetDir, cfg.layout)
	if err != nil {
		return fmt.Errorf("scan target directory: %w", err)
	}

	org := &TrackOrganizer{
		index:       index,
		layout:      cfg.layout,
		filenameOpt: FilenameOpts{IncludeTrackNumber: !cfg.NoTrackNumbers},
		result:      Result{},
		targetDir:   cfg.TargetDir,
		dryRun:      cfg.DryRun,
		verbose:     cfg.Verbose,
	}

	if cfg.DryRun {
		PrintDryRunWarning()
	}

	if !cfg.NoConsolidate {
		if err := org.consolidateExisting(); err != nil {
			return fmt.Errorf("consolidate existing duplicates: %w", err)
		}
	}

	var filePaths []string
	err = filepath.WalkDir(cfg.SourceDir, func(path string, d fs.DirEntry, err error) error {
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

// processFile processes a single audio file for organization. It uses the
// configured Layout to compute the components, then asks the TargetIndex to
// resolve each artist/album component to its canonical on-disk folder
// (matching existing case/edition variants when one is already present).
func (o *TrackOrganizer) processFile(path string) {
	meta, err := ReadFileMetadata(path)
	if err != nil {
		log.Printf("error reading tags for %s: %v", path, err)
		o.addResult(0, 0, 1)
		return
	}

	// Apply the artist-folder primary-name fix BEFORE the layout sees it,
	// so any tag with a `/` separator (e.g. "$uicideboy$/GERM") routes to
	// "$uicideboy$" rather than the Sanitize-mangled "$uicideboy$_GERM".
	// PartKindArtist is the only field this transformation should affect;
	// AlbumArtist gets the same treatment so the albumartist-album-year
	// layout benefits too.
	if primary := musicmatch.PrimaryArtistForFolder(meta.Artist); primary != "" {
		meta.Artist = primary
	}
	if primary := musicmatch.PrimaryArtistForFolder(meta.AlbumArtist); primary != "" {
		meta.AlbumArtist = primary
	}

	comps := o.layout.Components(meta)
	rawFilename := o.layout.Filename(meta, filepath.Ext(path), o.filenameOpt)
	sanitizedFilename := Sanitize(rawFilename)

	// Resolve each artist/album component against the index under one
	// lock so two goroutines don't both decide to create the same new
	// canonical folder name with different casings.
	o.indexMutex.Lock()
	resolved := o.resolveComponents(comps)
	o.indexMutex.Unlock()

	destDir := o.targetDir
	if len(resolved) > 0 {
		destDir = filepath.Join(append([]string{o.targetDir}, resolved...)...)
	}
	destPath := filepath.Join(destDir, sanitizedFilename)

	if path == destPath {
		o.addResult(0, 1, 0)
		return
	}
	if FileExists(destPath) {
		if o.verbose {
			PrintSkip(sanitizedFilename, "already exists")
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

	if err := CreateDirectoryPath(destDir); err != nil {
		log.Printf("error creating directory %s: %v", destDir, err)
		o.addResult(0, 0, 1)
		return
	}

	if FileExists(destPath) {
		if o.verbose {
			PrintSkip(sanitizedFilename, "already exists")
		}
		o.addResult(0, 1, 0)
		return
	}

	if o.verbose {
		PrintMove(sanitizedFilename)
	}
	if err := MoveFile(path, destPath, o.dryRun); err != nil {
		log.Printf("error moving %s: %v", path, err)
		o.addResult(0, 0, 1)
		return
	}
	o.addResult(1, 0, 0)
}

// resolveComponents converts the Layout's logical component list into the
// list of on-disk folder names this file should land in. For PartKindArtist
// and PartKindAlbum it consults the TargetIndex so case-fold and
// edition-variant matches reuse the existing folder; everything else is a
// straight Sanitize. Resolved artist and album names are also registered
// back into the index so the next file in this run sees the same
// canonical name even if its tags differ in casing.
func (o *TrackOrganizer) resolveComponents(comps []Component) []string {
	out := make([]string, 0, len(comps))
	resolvedArtist, resolvedAlbum := "", ""
	for _, c := range comps {
		switch c.Kind {
		case PartKindArtist:
			name := o.index.ResolveArtist(c.Raw)
			if name == "" {
				name = SanitizedComponentName(c)
			}
			resolvedArtist = name
			out = append(out, name)
		case PartKindAlbum:
			name := o.index.ResolveAlbum(resolvedArtist, c.Raw, c.Suffix)
			if name == "" {
				name = SanitizedComponentName(c)
			}
			resolvedAlbum = name
			out = append(out, name)
		default:
			out = append(out, SanitizedComponentName(c))
		}
	}
	if resolvedArtist != "" {
		o.index.RegisterIncoming(resolvedArtist, resolvedAlbum)
	}
	return out
}

func (o *TrackOrganizer) addResult(moved, skipped, errors int) {
	o.resultMutex.Lock()
	defer o.resultMutex.Unlock()
	o.result.Moved += moved
	o.result.Skipped += skipped
	o.result.Errors += errors
}

