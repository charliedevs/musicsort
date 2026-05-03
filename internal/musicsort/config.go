package musicsort

import (
	"fmt"
	"path/filepath"
)

// Config holds the configuration for the music organizer.
type Config struct {
	SourceDir string
	TargetDir string
	Recursive bool
	DryRun    bool
	Verbose   bool
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.SourceDir == "" {
		c.SourceDir = "."
	}
	if c.TargetDir == "" {
		c.TargetDir = "."
	}

	absSrc, err := filepath.Abs(c.SourceDir)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}
	c.SourceDir = absSrc

	absTgt, err := filepath.Abs(c.TargetDir)
	if err != nil {
		return fmt.Errorf("resolve target directory: %w", err)
	}
	c.TargetDir = absTgt

	return nil
}
