package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"musicsort/internal/flaghelp"
	"musicsort/internal/musicsort"
)

const version = "1.0.0"

func main() {
	cfg := musicsort.Config{}
	flaghelp.StringVar(&cfg.SourceDir, ".", "Source directory to scan", "-s", "--source")
	flaghelp.StringVar(&cfg.TargetDir, ".", "Target directory for organized files", "-t", "--target")
	flaghelp.BoolVar(&cfg.Recursive, false, "Enable recursive search", "-r", "--recursive")
	flaghelp.BoolVar(&cfg.DryRun, false, "Dry run (preview changes without moving)", "-n", "--dry-run")
	flaghelp.BoolVar(&cfg.Verbose, false, "Verbose output", "-v", "--verbose")
	help := false
	flaghelp.BoolVar(&help, false, "Show help", "-h", "--help")
	vers := false
	flaghelp.BoolVar(&vers, false, "Print version and exit", "--version")

	flag.Usage = flaghelp.Usage
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}
	if vers {
		fmt.Printf("musicsort version %s\n", version)
		os.Exit(0)
	}

	if err := musicsort.Run(cfg); err != nil {
		log.Fatalf("musicsort: %v", err)
	}
}
