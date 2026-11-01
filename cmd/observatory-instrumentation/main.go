package main

import (
	"fmt"
	"os"

	"github.com/emmett08/observatory-instrumentation/internal/config"
	"github.com/emmett08/observatory-instrumentation/internal/discovery"
	"github.com/emmett08/observatory-instrumentation/internal/installer"
)

var version = "development"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: observatory-instrumentation <init|adopt|resolve|discover|link-effort|version> [options]")
		os.Exit(2)
	}
	var err error
	switch args[0] {
	case "init", "adopt":
		err = installer.Run(args, os.Stdout, os.Stderr)
	case "resolve", "discover", "link-effort":
		err = discovery.Run(args, os.Stdout, os.Stderr)
	case "version":
		fmt.Printf("observatory-instrumentation %s; core %s\n", version, config.CoreRevision)
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "observatory-instrumentation:", err)
		os.Exit(1)
	}
}
