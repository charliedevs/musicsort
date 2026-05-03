package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"musicsort/internal/flaghelp"
	"musicsort/internal/spotifym3u"
)

const version = "1.0.0"

func main() {
	cfg := spotifym3u.Config{}
	flaghelp.StringVar(&cfg.CSVPath, "", "Path to Exportify/Spotify CSV file", "-c", "--csv")
	flaghelp.StringVar(&cfg.SourceDir, ".", "Source directory containing music files", "-s", "--source")
	flaghelp.StringVar(&cfg.OutputPath, "playlist.m3u", "Output M3U playlist path", "-o", "--output")
	flaghelp.StringVar(&cfg.TargetPrefix, "", "Target path prefix for playlist entries", "-p", "--target-prefix")
	flaghelp.BoolVar(&cfg.Recursive, false, "Scan source directory recursively", "-r", "--recursive")
	flaghelp.BoolVar(&cfg.DryRun, false, "Print summary without writing the playlist", "-n", "--dry-run")
	flaghelp.BoolVar(&cfg.Verbose, false, "Verbose output", "-v", "--verbose")
	flaghelp.BoolVar(&cfg.Debug, false, "Print debug details for unmatched tracks", "--debug")
	vers := false
	flaghelp.BoolVar(&vers, false, "Print version and exit", "--version")
	help := false
	flaghelp.BoolVar(&help, false, "Show help", "-h", "--help")

	flag.Usage = flaghelp.Usage
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}
	if vers {
		fmt.Printf("spotifym3u version %s\n", version)
		os.Exit(0)
	}

	if cfg.CSVPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -csv is required")
		flag.Usage()
		os.Exit(2)
	}

	if err := spotifym3u.Run(cfg); err != nil {
		log.Fatalf("spotifym3u: %v", err)
	}
}
