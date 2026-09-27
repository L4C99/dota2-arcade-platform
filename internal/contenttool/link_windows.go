//go:build windows

package contenttool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func resolveDirectory(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", err
	}
	if n >= uint32(len(buf)) {
		return "", fmt.Errorf("resolved path too long")
	}
	resolved := windows.UTF16ToString(buf[:n])
	if strings.HasPrefix(resolved, `\\?\UNC\`) {
		resolved = `\\` + strings.TrimPrefix(resolved, `\\?\UNC\`)
	} else {
		resolved = strings.TrimPrefix(resolved, `\\?\`)
	}
	return filepath.Clean(resolved), nil
}

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
