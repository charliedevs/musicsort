package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"musicsort/internal/flaghelp"
	"musicsort/internal/musicsort"
)

const version = "1.0.0"

func main() {
	cfg := musicsort.Config{}
	layoutUsage := "Layout preset (one of: " + strings.Join(musicsort.LayoutNames(), ", ") + ")"
	flaghelp.StringVar(&cfg.SourceDir, ".", "Source directory to scan", "-s", "--source")
	flaghelp.StringVar(&cfg.TargetDir, ".", "Target directory for organized files", "-t", "--target")
	flaghelp.StringVar(&cfg.LayoutName, musicsort.DefaultLayoutName, layoutUsage, "-l", "--layout")
	flaghelp.BoolVar(&cfg.Recursive, false, "Enable recursive search", "-r", "--recursive")
	flaghelp.BoolVar(&cfg.DryRun, false, "Dry run (preview changes without moving)", "-n", "--dry-run")
	flaghelp.BoolVar(&cfg.Verbose, false, "Verbose output", "-v", "--verbose")
	flaghelp.BoolVar(&cfg.NoTrackNumbers, false, "Do NOT prefix track numbers on filenames", "--no-track-numbers")
	flaghelp.BoolVar(&cfg.NoConsolidate, false, "Skip consolidating existing case/edition duplicate folders", "--no-consolidate")
	help := false
	flaghelp.BoolVar(&help, false, "Show help", "-h", "--help")
	vers := false
	flaghelp.BoolVar(&vers, false, "Print version and exit", "--version")

	flag.Usage = func() {
		flaghelp.Usage()
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprint(flag.CommandLine.Output(), musicsort.LayoutHelpText())
	}
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
