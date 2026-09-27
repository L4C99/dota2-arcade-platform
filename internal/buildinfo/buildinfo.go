// Package buildinfo exposes the version of a platform component.
package buildinfo

import (
	"encoding/json"
	"io"
	"runtime"
	"runtime/debug"
)

// Version is set by the build pipeline for a formal binary.
var Version = "dev"

// BuildTime is the UTC build timestamp supplied by the release pipeline.
var BuildTime = "unknown"

// Write emits machine-readable identity without contacting any service.
func Write(w io.Writer, component string) error {
	dirty := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.modified" {
				dirty = setting.Value
			}
		}
	}
	var gitDirty any
	if dirty == "true" || dirty == "false" {
		gitDirty = dirty == "true"
	}
	return json.NewEncoder(w).Encode(struct {
		Component string `json:"component"`
		Version   string `json:"version"`
		GitCommit string `json:"gitCommit"`
		BuildTime string `json:"buildTime"`
		GitDirty  any    `json:"gitDirty"`
		OS        string `json:"os"`
		Arch      string `json:"arch"`
		GoVersion string `json:"goVersion"`
	}{component, Version, Commit(), BuildTime, gitDirty, runtime.GOOS, runtime.GOARCH, runtime.Version()})
}

// Commit returns the source revision embedded by the Go toolchain, if any.
func Commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			return setting.Value
		}
	}
	return "unknown"
}
