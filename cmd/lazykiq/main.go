// Package main provides the lazykiq CLI entry point.
package main

import (
	"os"

	"github.com/kpumuk/lazykiq/internal/cmd"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
	BuiltBy = ""
)

func main() {
	if err := cmd.Execute(Version, Commit, Date, BuiltBy); err != nil {
		os.Exit(1)
	}
}
