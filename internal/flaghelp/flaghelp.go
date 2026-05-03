package flaghelp

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FlagGroup struct {
	Names   []string
	Typ     string
	Default string
	Usage   string
}

var groups []*FlagGroup

func StringVar(p *string, defaultValue string, usage string, names ...string) {
	for _, name := range names {
		flag.StringVar(p, normalizeName(name), defaultValue, usage)
	}
	groups = append(groups, &FlagGroup{Names: names, Typ: "string", Default: defaultValue, Usage: usage})
}

func BoolVar(p *bool, defaultValue bool, usage string, names ...string) {
	for _, name := range names {
		flag.BoolVar(p, normalizeName(name), defaultValue, usage)
	}
	groups = append(groups, &FlagGroup{Names: names, Typ: "bool", Default: fmt.Sprintf("%v", defaultValue), Usage: usage})
}

func normalizeName(name string) string {
	return strings.TrimLeft(name, "-")
}

func Usage() {
	exe := filepath.Base(os.Args[0])
	fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", exe)
	for _, group := range groups {
		names := strings.Join(group.Names, ", ")
		typeSpec := ""
		if group.Typ == "string" {
			typeSpec = " string"
		}
		fmt.Fprintf(flag.CommandLine.Output(), "  %s%s\n", names, typeSpec)
		defaultMsg := ""
		if group.Default != "" {
			defaultMsg = fmt.Sprintf(" (default %q)", group.Default)
		}
		fmt.Fprintf(flag.CommandLine.Output(), "      %s%s\n", group.Usage, defaultMsg)
	}
}
