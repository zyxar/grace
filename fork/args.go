// +build !js

package fork

import (
	"flag"
)

// GetArgs is an auxiliary function for Daemonize, for retrieving correct arguments
// to start a daemonized child process
func GetArgs(set *flag.FlagSet, filterFn func(string) bool) []string {
	if set == nil {
		set = flag.CommandLine
	}
	if set.NFlag() == 0 {
		return set.Args()
	}
	args := make([]string, 0, flag.NFlag()+flag.NArg())
	visit := func(f *flag.Flag) {
		args = append(args, "-"+f.Name+"="+f.Value.String())
	}
	if filterFn != nil {
		visit = func(f *flag.Flag) {
			if !filterFn(f.Name) {
				args = append(args, "-"+f.Name+"="+f.Value.String())
			}
		}
	}
	set.Visit(visit)
	return append(args, set.Args()...)
}
