package config

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TemplateManifestSHA256V1 hashes a local, explicitly enumerated and
// read-only version directory. Every referenced static startup dependency
// must be listed by the operator; a missing manifest never proves identity.
func (c Config) TemplateManifestSHA256V1(bindingKey string) (string, string, error) {
	binding, ok := c.TemplateManifests[bindingKey]
	if !ok || binding.VersionRoot == "" || binding.TemplatePath == "" {
		return "", "", errors.New("template manifest unavailable")
	}
	root, err := canonicalManifestPath(binding.VersionRoot)
	if err != nil {
		return "", "", err
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode().Perm()&0222 != 0 {
		return "", "", errors.New("version directory is not read-only")
	}
	template, err := canonicalManifestPath(binding.TemplatePath)
	if err != nil {
		return "", "", err
	}
	paths := append([]string{template}, binding.Dependencies...)
	entries := make([]manifestEntry, 0, len(paths))
	seen := make(map[string]bool)
	seenFiles := make([]os.FileInfo, 0, len(paths))
	for _, input := range paths {
		path, err := canonicalManifestPath(input)
		if err != nil {
			return "", "", err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", "", errors.New("manifest asset outside version root")
		}
		if seen[path] {
			return "", "", errors.New("duplicate manifest asset")
		}
		seen[path] = true
		for dir := filepath.Dir(path); dir != root; dir = filepath.Dir(dir) {
			if filepath.Dir(dir) == dir {
				return "", "", errors.New("unstable version root identity")
			}
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() || info.Mode().Perm()&0222 != 0 {
				return "", "", errors.New("writable or unavailable version subdirectory")
			}
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", "", err
		}
		for _, previous := range seenFiles {
			if os.SameFile(previous, info) {
				return "", "", errors.New("duplicate manifest file identity")
			}
		}
		seenFiles = append(seenFiles, info)
		entry, err := hashManifestAsset(path)
		if err != nil {
			return "", "", err
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	var templateEntry manifestEntry
	for _, entry := range entries {
		if entry.path == template {
			templateEntry = entry
			break
		}
	}
	h := sha256.New()
	writeManifestString(h, "template-manifest-sha256-v1")
	writeManifestString(h, filepath.ToSlash(template))
	writeManifestString(h, templateEntry.digest)
	// The template path and digest are encoded separately, followed by the
	// ordered dependency tuples. The template length is included as well.
	writeManifestUint(h, uint64(templateEntry.size))
	for _, entry := range entries {
		if entry.path == template {
			continue
		}
		writeManifestString(h, filepath.ToSlash(entry.path))
		writeManifestUint(h, uint64(entry.size))
		writeManifestString(h, entry.digest)
	}
	return template, hex.EncodeToString(h.Sum(nil)), nil
}

type manifestEntry struct {
	path, digest string
	size         int64
}

func canonicalManifestPath(path string) (string, error) {
	if err := validateCorePath(path); err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil || validateCorePath(canonical) != nil {
		return "", errors.New("unstable manifest path")
	}
	return filepath.Clean(canonical), nil
}

func hashManifestAsset(path string) (manifestEntry, error) {
	before, err := os.Stat(path)
	if err != nil || !before.Mode().IsRegular() {
		return manifestEntry{}, errors.New("manifest asset unavailable")
	}
	if before.Mode().Perm()&0222 != 0 {
		return manifestEntry{}, fmt.Errorf("manifest asset writable: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return manifestEntry{}, err
	}
	h := sha256.New()
	count, copyErr := io.Copy(h, file)
	closeErr := file.Close()
	after, statErr := os.Stat(path)
	if copyErr != nil || closeErr != nil || statErr != nil || count != before.Size() || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return manifestEntry{}, errors.New("manifest asset changed while hashing")
	}
	return manifestEntry{path: path, size: count, digest: hex.EncodeToString(h.Sum(nil))}, nil
}

func writeManifestString(w io.Writer, value string) {
	writeManifestUint(w, uint64(len(value)))
	_, _ = io.WriteString(w, value)
}
func writeManifestUint(w io.Writer, value uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], value)
	_, _ = w.Write(b[:])
}
