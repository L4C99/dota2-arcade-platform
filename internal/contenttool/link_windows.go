//go:build windows

package contenttool

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

// PowerShell receives paths through environment variables, never interpolated
// into script text. New-Item creates a directory Junction without symlink privilege.
func createDirectoryLink(link, target string) error {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`$ErrorActionPreference='Stop'; New-Item -ItemType Junction -Path $env:CONTENT_TOOL_LINK -Target $env:CONTENT_TOOL_TARGET | Out-Null`)
	cmd.Env = append(os.Environ(), "CONTENT_TOOL_LINK="+link, "CONTENT_TOOL_TARGET="+target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create junction: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func replaceFile(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func isDirectoryLink(path string) (bool, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return false, err
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 && attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0, nil
}
