// Package ownership gives one official Controller process exclusive ownership
// of a Node host's local d2core execution path for its entire lifetime.
package ownership

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var ErrHeld = errors.New("node Controller ownership already held")

type Lock struct {
	file *os.File
}

// Path uses the protected application data directory containing d2core's
// data directory. It never accepts a path from a Platform/Admin payload.
func Path(dataDir, nodeID string) (string, error) {
	if !filepath.IsAbs(dataDir) || nodeID == "" || strings.ContainsAny(nodeID, `/\:.`) || strings.ContainsRune(nodeID, 0) {
		return "", errors.New("invalid Controller ownership identity")
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(dataDir))
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", errors.New("Controller ownership directory unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0002 != 0 {
		return "", errors.New("Controller ownership directory is world-writable")
	}
	return filepath.Join(root, "controller-"+nodeID+".lock"), nil
}

// Acquire never waits. A duplicate supervisor fails before it can establish
// a capability session or enter the Runner. The caller retains Lock until exit.
func Acquire(dataDir, nodeID string) (*Lock, error) {
	path, err := Path(dataDir, nodeID)
	if err != nil {
		return nil, err
	}
	return acquire(path)
}

func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := release(l.file)
	l.file = nil
	return err
}
