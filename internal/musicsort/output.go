package musicsort

import "fmt"

// PrintDryRunWarning prints a warning that the operation is in dry-run mode.
func PrintDryRunWarning() {
	fmt.Println("\033[1;33m[DRY RUN] No files will actually be moved.\033[0m")
}

// PrintMove prints a message indicating a file was successfully moved.
func PrintMove(filename string) {
	fmt.Printf("\033[0;32m[MOVE]\033[0m %s\n", filename)
}

// PrintSkip prints a message indicating a file was skipped.
func PrintSkip(filename string, reason string) {
	fmt.Printf("\033[0;33m[SKIP]\033[0m %s (%s)\n", filename, reason)
}

// PrintDone prints a completion message.
func PrintDone() {
	fmt.Println("Done!")
}
