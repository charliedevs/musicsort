package clioutput

import "fmt"

const (
	Reset  = "\033[0m"
	Green  = "\033[0;32m"
	Yellow = "\033[0;33m"
	Red    = "\033[0;31m"
	Cyan   = "\033[0;36m"
	Bold   = "\033[1m"
)

func Colorize(text, color string) string {
	return color + text + Reset
}

func Label(text, color string) string {
	return Colorize("["+text+"]", color)
}

func Info(format string, a ...interface{}) {
	fmt.Printf(format, a...)
}

func InfoLine(format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
}

func ProgressLine(format string, a ...interface{}) {
	fmt.Printf("\r"+format, a...)
}

func Newline() {
	fmt.Println()
}

func SummaryHeader(title string) {
	fmt.Println(Colorize(title, Cyan))
}

func SummaryItem(label string, value interface{}) {
	fmt.Printf("  %s %v\n", Colorize(label, Bold), value)
}

func SummaryStatus(label string, value interface{}, color string) {
	fmt.Printf("  %s %v\n", Label(label, color), value)
}
