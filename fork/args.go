// +build !js

package fork

import (
	"flag"
)

// GetArgs is an auxiliary function for Daemonize, for retrieving correct arguments
// to start a daemonized child process.
// Default values would be skipped.
func GetArgs(set *flag.FlagSet, filterFn func(string) bool) []string {
	if set == nil {
		set = flag.CommandLine
	}
	if set.NFlag() == 0 {
		return set.Args()
	}
	args := make([]string, 0, flag.NFlag()+flag.NArg())
	visit := func(f *flag.Flag) {
		val := f.Value.String()
		if val != f.DefValue {
			args = append(args, "-"+f.Name+"="+val)
		}
	}
	if filterFn != nil {
		visit = func(f *flag.Flag) {
			if !filterFn(f.Name) {
				val := f.Value.String()
				if val != f.DefValue {
					args = append(args, "-"+f.Name+"="+val)
				}
			}
		}
	}
	set.Visit(visit)
	return append(args, set.Args()...)
}
