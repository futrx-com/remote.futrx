package main

import (
	"flag"
	"strings"
)

// permute reorders arguments so options may appear after positional arguments.
//
// Go's flag package stops at the first non-flag word, which would silently
// discard everything in "s3disk mount s3://bucket /mnt --endpoint http://…".
// Users reasonably expect GNU-style ordering, and quietly ignoring --endpoint
// would send requests to the wrong S3 service, so options are hoisted to the
// front before parsing.
func permute(fs *flag.FlagSet, args []string) []string {
	var options, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			positional = append(positional, arg)
			continue
		}
		options = append(options, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			continue // value is attached: --flag=value
		}
		f := fs.Lookup(name)
		if f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			options = append(options, args[i])
		}
	}
	return append(options, positional...)
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}
