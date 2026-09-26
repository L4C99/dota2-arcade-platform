// Package contenttool manages immutable local VPK releases. It has no Platform
// or d2core client and never changes lifecycle or drain state.
package contenttool

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var workshopPattern = regexp.MustCompile(`^[0-9]+$`)
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type Config struct {
	ContentRoot string
	DotaRoot    string
}

type Metadata struct {
	WorkshopID       string    `json:"workshopId"`
	ContentVersionID string    `json:"contentVersionId"`
	ReleasePath      string    `json:"releasePath"`
	VPKSHA256        string    `json:"vpkSha256"`
	SizeBytes        int64     `json:"sizeBytes"`
	PreparedAt       time.Time `json:"preparedAt"`
}

type Status struct {
	WorkshopID       string   `json:"workshopId"`
	CurrentVersion   string   `json:"currentVersion,omitempty"`
	PreviousVersion  string   `json:"previousVersion,omitempty"`
	PreparedVersions []string `json:"preparedVersions"`
	State            string   `json:"state"`
}

type pointer struct {
	Version string `json:"version"`
}

func (c Config) check() error {
	if !filepath.IsAbs(c.ContentRoot) || !filepath.IsAbs(c.DotaRoot) {
		return errors.New("content and Dota roots must be absolute")
	}
	for _, path := range []string{c.ContentRoot, c.DotaRoot} {
		for _, r := range path {
			if r > 127 || r < 32 || strings.ContainsRune(`[]*?`, r) {
				return errors.New("critical paths must be printable ASCII without wildcards")
			}
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			return fmt.Errorf("root is not a directory: %s", path)
		}
	}
	if filepath.Clean(c.ContentRoot) == filepath.Clean(c.DotaRoot) {
		return errors.New("content and Dota roots must differ")
	}
	return nil
}

func validID(workshop, version string) error {
	if !workshopPattern.MatchString(workshop) {
		return errors.New("invalid Workshop ID")
	}
	if version != "" && (!versionPattern.MatchString(version) || strings.EqualFold(version, "current") || strings.EqualFold(version, "previous") || strings.EqualFold(version, "pending")) {
		return errors.New("invalid version")
	}
	return nil
}

func (c Config) paths(workshop, version string) (base, release, meta, link string) {
	base = filepath.Join(c.ContentRoot, workshop)
	if version != "" {
		release = filepath.Join(base, "releases", version)
		meta = filepath.Join(base, "metadata", version+".json")
	}
	link = filepath.Join(c.DotaRoot, "game", "dota_addons", workshop)
	return
}

func realDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("expected real directory: %s", path)
	}
	return nil
}

func ensureDir(path string) error {
	if err := os.Mkdir(path, 0750); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return realDirectory(path)
}

func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return nil
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".content-meta-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0640); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return replaceFile(f.Name(), path)
}

func fileDigest(path string) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("VPK must be a regular file, not a link")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return "", 0, errors.New("VPK changed during read")
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func (c Config) readRelease(workshop, version string) (Metadata, error) {
	_, release, meta, _ := c.paths(workshop, version)
	var m Metadata
	if err := realDirectory(release); err != nil {
		return m, err
	}
	if err := readJSON(meta, &m); err != nil {
		return m, err
	}
	if m.WorkshopID != workshop || m.ContentVersionID != version || filepath.Clean(m.ReleasePath) != filepath.Clean(release) || m.SizeBytes <= 0 {
		return m, errors.New("release metadata does not match requested release")
	}
	hash, size, err := fileDigest(filepath.Join(release, "pak01_dir.vpk"))
	if err != nil {
		return m, err
	}
	if hash != m.VPKSHA256 || size != m.SizeBytes {
		return m, errors.New("release VPK differs from immutable metadata")
	}
	return m, nil
}

// Prepare creates a release without changing the Dota addon link or Platform.
func (c Config) Prepare(workshop, version, source string) (Metadata, error) {
	var empty Metadata
	if err := c.check(); err != nil {
		return empty, err
	}
	if err := validID(workshop, version); err != nil || version == "" {
		return empty, errors.New("Workshop ID and version are required")
	}
	if !filepath.IsAbs(source) {
		return empty, errors.New("source VPK path must be absolute")
	}
	hash, size, err := fileDigest(source)
	if err != nil {
		return empty, err
	}
	if size <= 0 {
		return empty, errors.New("source VPK is empty")
	}
	base, release, meta, _ := c.paths(workshop, version)
	if err := ensureDir(base); err != nil {
		return empty, err
	}
	if err := ensureDir(filepath.Join(base, "releases")); err != nil {
		return empty, err
	}
	if err := ensureDir(filepath.Join(base, "metadata")); err != nil {
		return empty, err
	}
	unlock, err := lockWorkshop(base)
	if err != nil {
		return empty, err
	}
	defer unlock()
	if _, err := os.Lstat(release); err == nil {
		m, readErr := c.readRelease(workshop, version)
		if errors.Is(readErr, os.ErrNotExist) {
			existingHash, existingSize, digestErr := fileDigest(filepath.Join(release, "pak01_dir.vpk"))
			if digestErr != nil || existingHash != hash || existingSize != size {
				return empty, errors.New("existing incomplete release differs from source")
			}
			m = Metadata{WorkshopID: workshop, ContentVersionID: version, ReleasePath: release, VPKSHA256: hash, SizeBytes: size, PreparedAt: time.Now().UTC()}
			if err := writeJSON(meta, m); err != nil {
				return empty, err
			}
			return c.readRelease(workshop, version)
		}
		if readErr != nil {
			return empty, fmt.Errorf("existing release is incomplete or different: %w", readErr)
		}
		if m.VPKSHA256 != hash || m.SizeBytes != size {
			return empty, errors.New("immutable version already has different content")
		}
		return m, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return empty, err
	}
	if _, err := os.Lstat(meta); err == nil {
		return empty, errors.New("metadata exists without release")
	} else if !errors.Is(err, os.ErrNotExist) {
		return empty, err
	}
	staging, err := os.MkdirTemp(filepath.Join(base, "releases"), ".prepare-*")
	if err != nil {
		return empty, err
	}
	defer os.RemoveAll(staging)
	in, err := os.Open(source)
	if err != nil {
		return empty, err
	}
	out, err := os.OpenFile(filepath.Join(staging, "pak01_dir.vpk"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		in.Close()
		return empty, err
	}
	copyHash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, copyHash), in)
	closeOutErr := out.Close()
	closeInErr := in.Close()
	if copyErr != nil {
		return empty, copyErr
	}
	if closeOutErr != nil {
		return empty, closeOutErr
	}
	if closeInErr != nil {
		return empty, closeInErr
	}
	if written != size || hex.EncodeToString(copyHash.Sum(nil)) != hash {
		return empty, errors.New("source VPK changed during copy")
	}
	if err := os.Rename(staging, release); err != nil {
		return empty, err
	}
	m := Metadata{WorkshopID: workshop, ContentVersionID: version, ReleasePath: release, VPKSHA256: hash, SizeBytes: size, PreparedAt: time.Now().UTC()}
	if err := writeJSON(meta, m); err != nil {
		return empty, fmt.Errorf("release prepared but metadata write failed; inspect before retry: %w", err)
	}
	if _, err := c.readRelease(workshop, version); err != nil {
		return empty, err
	}
	return m, nil
}

func uniqueSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (c Config) current(workshop string) (string, error) {
	base, _, _, link := c.paths(workshop, "")
	var m Metadata
	if err := readJSON(filepath.Join(base, "metadata", "current.json"), &m); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, linkErr := os.Lstat(link); errors.Is(linkErr, os.ErrNotExist) {
				return "", nil
			}
		}
		return "", fmt.Errorf("current metadata is absent or invalid: %w", err)
	}
	if err := validID(workshop, m.ContentVersionID); err != nil || m.ContentVersionID == "" || m.WorkshopID != workshop {
		return "", errors.New("invalid current metadata")
	}
	_, release, _, _ := c.paths(workshop, m.ContentVersionID)
	prepared, err := c.readRelease(workshop, m.ContentVersionID)
	if err != nil {
		return "", err
	}
	if m.VPKSHA256 != prepared.VPKSHA256 || m.SizeBytes != prepared.SizeBytes || filepath.Clean(m.ReleasePath) != filepath.Clean(prepared.ReleasePath) {
		return "", errors.New("current metadata differs from release")
	}
	linked, err := isDirectoryLink(link)
	if err != nil {
		return "", err
	}
	if !linked {
		return "", errors.New("current addon is not a directory link")
	}
	linkInfo, err := os.Stat(link)
	if err != nil {
		return "", err
	}
	releaseInfo, err := os.Stat(release)
	if err != nil {
		return "", err
	}
	if !os.SameFile(linkInfo, releaseInfo) {
		return "", errors.New("current link and metadata disagree")
	}
	return m.ContentVersionID, nil
}

func (c Config) Status(workshop string) (Status, error) {
	s := Status{WorkshopID: workshop, PreparedVersions: []string{}, State: "unknown"}
	if err := c.check(); err != nil {
		return s, err
	}
	if err := validID(workshop, ""); err != nil {
		return s, err
	}
	base, _, _, _ := c.paths(workshop, "")
	entries, err := os.ReadDir(filepath.Join(base, "metadata"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return s, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || name == "current.json" || name == "previous.json" || name == "pending.json" {
			continue
		}
		version := strings.TrimSuffix(name, ".json")
		if validID(workshop, version) == nil {
			s.PreparedVersions = append(s.PreparedVersions, version)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "metadata", "pending.json")); err == nil {
		s.State = "transition_incomplete"
		return s, nil
	}
	current, err := c.current(workshop)
	if err != nil {
		return s, err
	}
	s.CurrentVersion = current
	if current == "" {
		s.State = "not_switched"
	} else {
		s.State = "confirmed"
	}
	var previous pointer
	if readJSON(filepath.Join(base, "metadata", "previous.json"), &previous) == nil {
		s.PreviousVersion = previous.Version
	}
	return s, nil
}

// Switch changes only the directory link and Controller-readable metadata.
// The operator must have drained the content before invoking it.
func (c Config) Switch(workshop, version string) error {
	return c.switchVersion(workshop, version, false)
}

func (c Config) switchVersion(workshop, version string, rollback bool) error {
	if err := c.check(); err != nil {
		return err
	}
	if err := validID(workshop, version); err != nil || !rollback && version == "" {
		return errors.New("invalid Workshop ID or version")
	}
	base, _, _, link := c.paths(workshop, version)
	if err := realDirectory(filepath.Join(base, "metadata")); err != nil {
		return err
	}
	unlock, err := lockWorkshop(base)
	if err != nil {
		return err
	}
	defer unlock()
	if err := c.recoverTransition(workshop); err != nil {
		return fmt.Errorf("recover previous content transition: %w", err)
	}
	if rollback {
		var p pointer
		if err := readJSON(filepath.Join(base, "metadata", "previous.json"), &p); err != nil {
			return err
		}
		version = p.Version
		if version == "" || validID(workshop, version) != nil {
			return errors.New("no valid previous version")
		}
	}
	metadata, err := c.readRelease(workshop, version)
	if err != nil {
		return err
	}
	old, err := c.current(workshop)
	if err != nil {
		return err
	}
	if old == version {
		if rollback {
			_ = os.Remove(filepath.Join(c.ContentRoot, workshop, "metadata", "previous.json"))
		}
		return nil
	}
	_, release, _, _ := c.paths(workshop, version)
	addonDir := filepath.Dir(link)
	if err := realDirectory(addonDir); err != nil {
		return err
	}
	suffix, err := uniqueSuffix()
	if err != nil {
		return err
	}
	tempLink := filepath.Join(addonDir, ".content-tool-"+workshop+"-"+suffix)
	backupLink := filepath.Join(addonDir, ".content-tool-backup-"+workshop+"-"+suffix)
	if err := createDirectoryLink(tempLink, release); err != nil {
		return err
	}
	pending := transition{Old: old, New: version, TempName: filepath.Base(tempLink), BackupName: filepath.Base(backupLink), Rollback: rollback}
	if err := writeJSON(filepath.Join(base, "metadata", "pending.json"), pending); err != nil {
		_ = os.Remove(tempLink)
		return err
	}
	if old != "" {
		if err := os.Rename(link, backupLink); err != nil {
			_ = c.recoverTransition(workshop)
			return err
		}
	}
	if err := os.Rename(tempLink, link); err != nil {
		_ = c.recoverTransition(workshop)
		return err
	}
	if err := c.recoverTransition(workshop); err != nil {
		return err
	}
	current, err := c.current(workshop)
	if err != nil {
		return fmt.Errorf("switch readback: %w", err)
	}
	if current != version {
		return fmt.Errorf("switch readback found %q, expected %q", current, version)
	}
	if metadata.ContentVersionID != version {
		return errors.New("invalid prepared metadata")
	}
	return nil
}

func (c Config) Rollback(workshop string) error {
	return c.switchVersion(workshop, "", true)
}
