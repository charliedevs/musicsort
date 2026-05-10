package musicsort

import (
	"fmt"
	"path/filepath"
)

// Config holds the configuration for the music organizer.
type Config struct {
	SourceDir       string
	TargetDir       string
	LayoutName      string
	Recursive       bool
	DryRun          bool
	Verbose         bool
	NoTrackNumbers  bool
	NoConsolidate   bool

	// layout is the resolved Layout corresponding to LayoutName, populated
	// by Validate. Callers should use this rather than re-resolving by name
	// per file.
	layout *Layout
}

// Layout returns the resolved Layout pointer; callers must invoke Validate
// first. Returns nil if the config has not been validated.
func (c *Config) Layout() *Layout { return c.layout }

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

	if c.LayoutName == "" {
		c.LayoutName = DefaultLayoutName
	}
	layout, err := LayoutByName(c.LayoutName)
	if err != nil {
		return err
	}
	c.layout = layout

	return nil
}
