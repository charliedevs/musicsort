// Package clioutput provides a small wrapper around fmt for printing colored
// labels, progress lines, and summary blocks to stdout. Color and progress
// (carriage-return) output are auto-disabled when stdout is not a terminal
// or when the NO_COLOR environment variable is set.
package clioutput

import (
	"fmt"
	"os"
)

const (
	Reset  = "\033[0m"
	Green  = "\033[0;32m"
	Yellow = "\033[0;33m"
	Red    = "\033[0;31m"
	Cyan   = "\033[0;36m"
	Bold   = "\033[1m"
)

// useColor controls whether ANSI color codes are emitted.
// useProgress controls whether \r-style in-place progress lines are emitted.
// Both are determined once at process start; tests can override via the
// SetColorEnabled / SetProgressEnabled helpers if ever needed.
var (
	useColor    = detectColor()
	useProgress = detectProgress()
)

func detectColor() bool {
	if v, ok := os.LookupEnv("NO_COLOR"); ok && v != "" {
		return false
	}
	return isStdoutTerminal()
}

func detectProgress() bool { return isStdoutTerminal() }

func isStdoutTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// SetColorEnabled overrides the auto-detected color setting. Intended for
// tests or future flags.
func SetColorEnabled(on bool) { useColor = on }

// SetProgressEnabled overrides the auto-detected progress setting.
func SetProgressEnabled(on bool) { useProgress = on }

// ProgressEnabled reports whether ProgressLine will actually emit output.
// Callers use this to decide whether the trailing Newline that "ends" a
// progress section is meaningful or just noise.
func ProgressEnabled() bool { return useProgress }

// Colorize wraps text in an ANSI color sequence, or returns it unchanged when
// color output is disabled.
func Colorize(text, color string) string {
	if !useColor {
		return text
	}
	return color + text + Reset
}

// Label renders text as a bracketed, colored tag, e.g. "[MATCH]".
func Label(text, color string) string {
	return Colorize("["+text+"]", color)
}

// Info writes a printf-style message to stdout without a trailing newline.
func Info(format string, a ...any) {
	fmt.Printf(format, a...)
}

// InfoLine writes a printf-style message to stdout followed by a newline.
func InfoLine(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

// ProgressLine prints a progress message that overwrites itself in place when
// stdout is a terminal. When stdout is not a terminal the call is a no-op so
// progress noise doesn't pollute pipes, log files, or CI output.
func ProgressLine(format string, a ...any) {
	if !useProgress {
		return
	}
	fmt.Printf("\r"+format, a...)
}

// Newline prints a single newline. Useful for separating phases.
func Newline() {
	fmt.Println()
}

// SummaryHeader prints a colored header for a summary block.
func SummaryHeader(title string) {
	fmt.Println(Colorize(title, Cyan))
}

// SummaryItem prints a labelled value as part of a summary block.
func SummaryItem(label string, value any) {
	fmt.Printf("  %s %v\n", Colorize(label, Bold), value)
}

// SummaryStatus prints a colored bracketed label with a value, e.g.
// "[Matched:] 42".
func SummaryStatus(label string, value any, color string) {
	fmt.Printf("  %s %v\n", Label(label, color), value)
}
