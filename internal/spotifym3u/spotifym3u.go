// Package spotifym3u turns a Spotify/Exportify CSV export into an M3U
// playlist that points at a local music library. The pipeline is:
//
//  1. Parse the CSV into PlaylistEntry rows.
//  2. Walk the source directory and build a TrackIndex of audio files,
//     capturing both raw and normalized metadata.
//  3. Match every entry against the index using a tiered scoring strategy
//     (see match.go).
//  4. Write the matched results as an extended M3U file (or skip the write
//     in --dry-run mode).
//  5. Print a summary, optionally followed by a debug breakdown of misses.
package spotifym3u

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"musicsort/internal/clioutput"
)

// Config holds the user-supplied options for a single Run invocation. Field
// defaults are applied at the top of Run.
type Config struct {
	CSVPath      string
	SourceDir    string
	OutputPath   string
	TargetPrefix string
	Recursive    bool
	DryRun       bool
	Verbose      bool
	Debug        bool
}

// Run executes the full CSV → playlist pipeline. It returns an error only
// for fatal conditions (missing CSV, unreadable source directory, write
// failure); per-track misses are reported via the printed summary.
func Run(cfg Config) error {
	if cfg.CSVPath == "" {
		return fmt.Errorf("csv path is required")
	}
	if cfg.SourceDir == "" {
		cfg.SourceDir = "."
	}
	if cfg.OutputPath == "" {
		cfg.OutputPath = "playlist.m3u"
	}
	cfg.TargetPrefix = trimTrailingSeparators(cfg.TargetPrefix)

	absSource, err := filepath.Abs(cfg.SourceDir)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}

	rows, err := ParseSpotifyCSV(cfg.CSVPath)
	if err != nil {
		return fmt.Errorf("parse csv: %w", err)
	}

	indexStart := time.Now()
	index, err := BuildIndex(absSource, cfg.Recursive, cfg.Verbose)
	if err != nil {
		return fmt.Errorf("build source index: %w", err)
	}
	indexElapsed := time.Since(indexStart)

	matchStart := time.Now()
	result := index.MatchPlaylist(rows, cfg.Verbose)
	matchElapsed := time.Since(matchStart)

	if !cfg.DryRun {
		if err := WritePlaylist(cfg.OutputPath, cfg.TargetPrefix, result.Matches); err != nil {
			return fmt.Errorf("write playlist: %w", err)
		}
	}

	if cfg.Verbose {
		clioutput.Newline()
	}
	PrintSummary(cfg.OutputPath, cfg.TargetPrefix, result, cfg.DryRun, cfg.Debug)
	clioutput.SummaryItem("Elapsed:", fmt.Sprintf("%s indexing, %s matching",
		roundDuration(indexElapsed), roundDuration(matchElapsed)))
	return nil
}

// trimTrailingSeparators strips trailing path separators (both Unix `/` and
// the platform-native one) from prefix so target-prefix joins don't produce
// double slashes.
func trimTrailingSeparators(prefix string) string {
	cutset := "/"
	if string(filepath.Separator) != "/" {
		cutset += string(filepath.Separator)
	}
	return strings.TrimRight(prefix, cutset)
}

// roundDuration rounds d to a unit appropriate for human display: ms for
// short runs, hundredths of a second otherwise.
func roundDuration(d time.Duration) time.Duration {
	if d < time.Second {
		return d.Round(time.Millisecond)
	}
	return d.Round(10 * time.Millisecond)
}
