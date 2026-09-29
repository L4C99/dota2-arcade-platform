//go:build linux

package ownership

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func acquire(path string) (*Lock, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrHeld
		}
		return nil, err
	}
	return &Lock{file: file}, nil
}

func release(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
