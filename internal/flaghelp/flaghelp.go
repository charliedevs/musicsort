// Package flaghelp wraps the standard flag package so a single flag can be
// registered under multiple names (a short and a long form, for example) and
// rendered in a more human-readable Usage block than flag.PrintDefaults.
package flaghelp

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FlagGroup describes a single flag with all of its registered aliases. It is
// used only to render the Usage block; flag values themselves are stored in
// the standard flag.FlagSet.
type FlagGroup struct {
	Names   []string
	Typ     string
	Default string
	Usage   string
}

var groups []*FlagGroup

// StringVar registers a string flag under one or more names (short and long
// forms) and remembers the group for the custom Usage renderer.
func StringVar(p *string, defaultValue string, usage string, names ...string) {
	for _, name := range names {
		flag.StringVar(p, normalizeName(name), defaultValue, usage)
	}
	groups = append(groups, &FlagGroup{Names: names, Typ: "string", Default: defaultValue, Usage: usage})
}

// BoolVar registers a boolean flag under one or more names.
func BoolVar(p *bool, defaultValue bool, usage string, names ...string) {
	for _, name := range names {
		flag.BoolVar(p, normalizeName(name), defaultValue, usage)
	}
	groups = append(groups, &FlagGroup{Names: names, Typ: "bool", Default: fmt.Sprintf("%v", defaultValue), Usage: usage})
}

// normalizeName strips the leading dashes the user passed in ("-c", "--csv")
// so the underlying flag package sees the bare name ("c", "csv").
func normalizeName(name string) string {
	return strings.TrimLeft(name, "-")
}

// Usage prints the registered flag groups in the order they were registered.
// Bool flags whose default is false render without a "(default ...)" tail to
// keep the help screen quiet for the common case.
func Usage() {
	exe := filepath.Base(os.Args[0])
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "Usage of %s:\n", exe)
	for _, group := range groups {
		names := strings.Join(group.Names, ", ")
		typeSpec := ""
		if group.Typ == "string" {
			typeSpec = " string"
		}
		fmt.Fprintf(out, "  %s%s\n", names, typeSpec)
		fmt.Fprintf(out, "      %s%s\n", group.Usage, defaultSuffix(group))
	}
}

// defaultSuffix returns " (default ...)" when there's a meaningful default to
// surface, and "" otherwise. Bool flags default to false silently because
// "(default false)" on every help-screen flag is just noise.
func defaultSuffix(g *FlagGroup) string {
	if g.Typ == "bool" && g.Default == "false" {
		return ""
	}
	if g.Default == "" {
		return ""
	}
	return fmt.Sprintf(" (default %q)", g.Default)
}
