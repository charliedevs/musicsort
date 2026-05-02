package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"musicsort/internal/spotifym3u"
)

func main() {
	cfg := spotifym3u.Config{}
	flag.StringVar(&cfg.CSVPath, "csv", "", "Path to Exportify/Spotify CSV file")
	flag.StringVar(&cfg.SourceDir, "source", ".", "Source directory containing music files")
	flag.StringVar(&cfg.OutputPath, "output", "playlist.m3u", "Output M3U playlist path")
	flag.StringVar(&cfg.TargetPrefix, "target-prefix", "", "Target path prefix for playlist entries")
	flag.BoolVar(&cfg.Recursive, "recursive", false, "Scan source directory recursively")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Print summary without writing the playlist")
	flag.BoolVar(&cfg.Debug, "debug", false, "Print debug details for unmatched tracks")
	flag.Parse()

	if cfg.CSVPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -csv is required")
		flag.Usage()
		os.Exit(2)
	}

	if err := spotifym3u.Run(cfg); err != nil {
		log.Fatalf("spotifym3u: %v", err)
	}
}
