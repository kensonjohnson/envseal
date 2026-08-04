package main

import (
	"os"

	"github.com/kensonjohnson/envseal/internal/cli"
)

// version is replaced by the release workflow with -ldflags.
var version = "devel"

func main() {
	os.Exit(cli.Run(os.Args[1:], version, os.Stdout, os.Stderr))
}
