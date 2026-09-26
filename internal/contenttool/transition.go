package contenttool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type transition struct {
	Old        string `json:"old"`
	New        string `json:"new"`
	TempName   string `json:"tempName"`
	BackupName string `json:"backupName"`
	Rollback   bool   `json:"rollback"`
}

// A leftover lock after a process crash requires operator inspection. It is
// never timed out automatically, because two content operations must not race.
func lockWorkshop(base string) (func(), error) {
	path := filepath.Join(base, "metadata", ".content-tool.lock")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("content operation locked; inspect previous process before removing lock: %w", err)
	}
	_, _ = fmt.Fprintf(f, "pid=%d\n", os.Getpid())
	_ = f.Close()
	return func() { _ = os.Remove(path) }, nil
}

func sameLinkedDirectory(link, release string) (bool, error) {
	linked, err := isDirectoryLink(link)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !linked {
		return false, errors.New("unknown real directory blocks content recovery")
	}
	a, err := os.Stat(link)
	if err != nil {
		return false, err
	}
	b, err := os.Stat(release)
	if err != nil {
		return false, err
	}
	return os.SameFile(a, b), nil
}

func removeKnownLink(link, release string) error {
	match, err := sameLinkedDirectory(link, release)
	if err != nil {
		return err
	}
	if !match {
		if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errors.New("temporary content link target changed")
	}
	return os.Remove(link)
}

func (c Config) recoverTransition(workshop string) error {
	base, _, _, link := c.paths(workshop, "")
	path := filepath.Join(base, "metadata", "pending.json")
	var p transition
	if err := readJSON(path, &p); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if p.New == "" || validID(workshop, p.New) != nil || p.Old != "" && validID(workshop, p.Old) != nil ||
		filepath.Base(p.TempName) != p.TempName || filepath.Base(p.BackupName) != p.BackupName ||
		!strings.HasPrefix(p.TempName, ".content-tool-"+workshop+"-") ||
		!strings.HasPrefix(p.BackupName, ".content-tool-backup-"+workshop+"-") {
		return errors.New("invalid content transition record")
	}
	_, newRelease, _, _ := c.paths(workshop, p.New)
	addonDir := filepath.Dir(link)
	temp := filepath.Join(addonDir, p.TempName)
	backup := filepath.Join(addonDir, p.BackupName)
	newCurrent, err := sameLinkedDirectory(link, newRelease)
	if err != nil {
		return err
	}
	if newCurrent {
		metadata, err := c.readRelease(workshop, p.New)
		if err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(base, "metadata", "current.json"), metadata); err != nil {
			return err
		}
		if p.Rollback || p.Old == "" {
			_ = os.Remove(filepath.Join(base, "metadata", "previous.json"))
		} else if err := writeJSON(filepath.Join(base, "metadata", "previous.json"), pointer{Version: p.Old}); err != nil {
			return err
		}
		if p.Old != "" {
			_, oldRelease, _, _ := c.paths(workshop, p.Old)
			if err := removeKnownLink(backup, oldRelease); err != nil {
				return err
			}
		}
		if err := removeKnownLink(temp, newRelease); err != nil {
			return err
		}
		return os.Remove(path)
	}
	if p.Old != "" {
		_, oldRelease, _, _ := c.paths(workshop, p.Old)
		oldCurrent, err := sameLinkedDirectory(link, oldRelease)
		if err != nil {
			return err
		}
		if !oldCurrent {
			backupMatches, err := sameLinkedDirectory(backup, oldRelease)
			if err != nil {
				return err
			}
			if !backupMatches {
				return errors.New("content transition lost both current and backup links")
			}
			if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
				return errors.New("unknown addon link blocks recovery")
			}
			if err := os.Rename(backup, link); err != nil {
				return err
			}
		}
	} else if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		return errors.New("unknown initial addon link blocks recovery")
	}
	if err := removeKnownLink(temp, newRelease); err != nil {
		return err
	}
	return os.Remove(path)
}
