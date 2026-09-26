//go:build !windows

package contenttool

import "os"

func createDirectoryLink(link, target string) error { return os.Symlink(target, link) }
func replaceFile(from, to string) error             { return os.Rename(from, to) }
func isDirectoryLink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}
