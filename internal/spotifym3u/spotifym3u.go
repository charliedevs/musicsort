package spotifym3u

import (
	"fmt"
	"path/filepath"
	"strings"

	"musicsort/internal/clioutput"
)

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
	cfg.TargetPrefix = strings.TrimRight(cfg.TargetPrefix, string(filepath.Separator))
	cfg.TargetPrefix = strings.TrimRight(cfg.TargetPrefix, "/")

	absSource, err := filepath.Abs(cfg.SourceDir)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}

	rows, err := ParseSpotifyCSV(cfg.CSVPath)
	if err != nil {
		return fmt.Errorf("parse csv: %w", err)
	}

	index, err := BuildIndex(absSource, cfg.Recursive, cfg.Verbose)
	if err != nil {
		return fmt.Errorf("build source index: %w", err)
	}

	result := index.MatchPlaylist(rows, cfg.Verbose)

	if !cfg.DryRun {
		if err := WritePlaylist(cfg.OutputPath, cfg.TargetPrefix, result.Matches); err != nil {
			return fmt.Errorf("write playlist: %w", err)
		}
	}

	if cfg.Verbose {
		clioutput.Newline()
	}
	PrintSummary(cfg.OutputPath, cfg.TargetPrefix, result, cfg.DryRun, cfg.Debug)
	return nil
}
