package main

import (
	"fmt"
	"os"

	"github.com/L4C99/dota2-arcade-platform/internal/buildinfo"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Printf("platform-server %s (%s)\n", buildinfo.Version, buildinfo.Commit())
		return
	}
	fmt.Fprintln(os.Stderr, "platform-server: P0A build baseline; server startup is added in P0B")
	os.Exit(2)
}
