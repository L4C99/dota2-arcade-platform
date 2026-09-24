// Package buildinfo exposes the version of a platform component.
package buildinfo

import "runtime/debug"

// Version is set by the build pipeline for a formal binary.
var Version = "dev"

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
