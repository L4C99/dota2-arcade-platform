package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/L4C99/dota2-arcade-platform/internal/contenttool"
)

func main() {
	contentRoot := flag.String("content-root", os.Getenv("CONTENT_ROOT"), "absolute content repository root")
	dotaRoot := flag.String("dota-root", os.Getenv("DOTA_ROOT"), "absolute Dota installation root")
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		usage()
	}
	tool := contenttool.Config{ContentRoot: *contentRoot, DotaRoot: *dotaRoot}
	var result any
	var err error
	switch args[0] {
	case "status":
		if len(args) != 2 {
			usage()
		}
		result, err = tool.Status(args[1])
	case "prepare":
		if len(args) != 4 {
			usage()
		}
		result, err = tool.Prepare(args[1], args[2], args[3])
	case "switch":
		if len(args) != 3 {
			usage()
		}
		err = tool.Switch(args[1], args[2])
		if err == nil {
			result, err = tool.Status(args[1])
		}
	case "rollback":
		if len(args) != 2 {
			usage()
		}
		err = tool.Rollback(args[1])
		if err == nil {
			result, err = tool.Status(args[1])
		}
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "content-tool:", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: content-tool [--content-root ABS] [--dota-root ABS] status WORKSHOP_ID | prepare WORKSHOP_ID VERSION SOURCE_VPK | switch WORKSHOP_ID VERSION | rollback WORKSHOP_ID")
	os.Exit(2)
}
