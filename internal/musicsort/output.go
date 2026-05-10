package musicsort

import "musicsort/internal/clioutput"

// PrintDryRunWarning prints a warning that the operation is in dry-run mode.
func PrintDryRunWarning() {
	clioutput.InfoLine("%s No files will actually be moved.", clioutput.Label("DRY RUN", clioutput.Yellow))
}

// PrintMove prints a message indicating a file was successfully moved.
func PrintMove(filename string) {
	clioutput.InfoLine("%s %s", clioutput.Label("MOVE", clioutput.Green), filename)
}

// PrintSkip prints a message indicating a file was skipped.
func PrintSkip(filename string, reason string) {
	clioutput.InfoLine("%s %s (%s)", clioutput.Label("SKIP", clioutput.Yellow), filename, reason)
}

// PrintSummary prints a colored summary of the organization operation.
// Consolidation counters are only surfaced when non-zero so a clean run
// against a tidy library doesn't pad the summary with empty rows.
func PrintSummary(result Result, dryRun bool) {
	clioutput.Newline()
	clioutput.SummaryHeader("Summary")
	clioutput.SummaryItem("Total:", result.Total)
	clioutput.SummaryStatus("Moved:", result.Moved, clioutput.Green)
	clioutput.SummaryStatus("Skipped:", result.Skipped, clioutput.Yellow)
	clioutput.SummaryStatus("Errors:", result.Errors, clioutput.Red)
	if result.Consolidated > 0 {
		clioutput.SummaryStatus("Consolidated:", result.Consolidated, clioutput.Cyan)
	}
	if result.RenamedOnMerge > 0 {
		clioutput.SummaryStatus("Renamed on merge:", result.RenamedOnMerge, clioutput.Yellow)
	}
	if dryRun {
		clioutput.SummaryItem("Status:", "dry run (no files were moved)")
	} else {
		clioutput.SummaryItem("Status:", "completed successfully")
	}
}
