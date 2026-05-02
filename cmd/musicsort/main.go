package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"musicsort/internal/musicsort"
)

const version = "1.0.0"

func main() {
	cfg := musicsort.Config{}
	flag.StringVar(&cfg.SourceDir, "s", ".", "Source directory to scan")
	flag.StringVar(&cfg.TargetDir, "t", ".", "Target directory for organized files")
	flag.BoolVar(&cfg.Recursive, "r", false, "Enable recursive search")
	flag.BoolVar(&cfg.DryRun, "n", false, "Dry run (preview changes without moving)")
	vers := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *vers {
		fmt.Printf("musicsort version %s\n", version)
		os.Exit(0)
	}

	if err := musicsort.Run(cfg); err != nil {
		log.Fatalf("musicsort: %v", err)
	}
}
