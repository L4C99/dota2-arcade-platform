//go:build windows

package ownership

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func acquire(path string) (*Lock, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Controller ownership lock path is a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	var overlap windows.Overlapped
	err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlap)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrHeld
		}
		return nil, err
	}
	return &Lock{file: file}, nil
}

func release(file *os.File) error {
	var overlap windows.Overlapped
	err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlap)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
