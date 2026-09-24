package main

import (
	"fmt"
	"os"

	"github.com/L4C99/dota2-arcade-platform/internal/buildinfo"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Printf("node-controller %s (%s)\n", buildinfo.Version, buildinfo.Commit())
		return
	}
	fmt.Fprintln(os.Stderr, "node-controller: P0A build baseline; controller startup is added in P0C")
	os.Exit(2)
}
