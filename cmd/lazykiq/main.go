// Package main provides the lazykiq CLI entry point.
package main

import "os"

func main() {
	if err := Execute(); err != nil {
		os.Exit(1)
	}
}
