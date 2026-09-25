package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

type buildManifest struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
}

type contentMetadata struct {
	WorkshopID       string `json:"workshopId"`
	ContentVersionID string `json:"contentVersionId"`
	ReleasePath      string `json:"releasePath"`
	VPKSHA256        string `json:"vpkSha256"`
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type cachedVPK struct {
	info   os.FileInfo
	digest string
}

type FactReader struct {
	config Config
	cache  map[string]cachedVPK
}

func NewFactReader(c Config) *FactReader {
	return &FactReader{config: c, cache: make(map[string]cachedVPK)}
}

// Facts reports only local files that the Controller can read back. Core
// protocolVersion remains zero until P0D establishes the local client link.
func (c Config) Facts(controllerVersion string, coreProtocolVersion int) nodev1.Heartbeat {
	return NewFactReader(c).Facts(controllerVersion, coreProtocolVersion)
}

func (r *FactReader) Facts(controllerVersion string, coreProtocolVersion int) nodev1.Heartbeat {
	c := r.config
	h := nodev1.Heartbeat{OS: runtime.GOOS, ControllerVersion: controllerVersion, NodeAPIVersion: nodev1.APIVersion,
		D2CoreVersion: "unknown", D2CoreCommit: "unknown", D2CoreProtocolVersion: coreProtocolVersion,
		HardMaxInstances: c.HardMaxInstances, Network: c.Network, Content: make([]nodev1.ContentFact, 0, len(c.ContentBindings))}
	if raw, err := os.ReadFile(c.D2CoreBuildFile); err == nil {
		var manifest buildManifest
		if json.Unmarshal(raw, &manifest) == nil && manifest.Version != "" && manifest.GitCommit != "" {
			h.D2CoreVersion = manifest.Version
			h.D2CoreCommit = manifest.GitCommit
		}
	}
	for _, binding := range c.ContentBindings {
		h.Content = append(h.Content, r.readContentFact(binding))
	}
	return h
}

func (r *FactReader) readContentFact(binding ContentBinding) nodev1.ContentFact {
	fact := nodev1.ContentFact{WorkshopID: binding.WorkshopID, State: "unknown"}
	raw, err := os.ReadFile(binding.MetadataPath)
	if err != nil {
		return fact
	}
	var metadata contentMetadata
	if json.Unmarshal(raw, &metadata) != nil || metadata.WorkshopID != binding.WorkshopID ||
		metadata.ContentVersionID == "" || !filepath.IsAbs(metadata.ReleasePath) || !sha256Pattern.MatchString(metadata.VPKSHA256) {
		return fact
	}
	linkInfo, err := os.Stat(binding.CurrentLinkPath)
	if err != nil {
		return fact
	}
	releaseInfo, err := os.Stat(metadata.ReleasePath)
	if err != nil {
		return fact
	}
	// Stat follows both Unix symlinks and Windows junctions. On some Windows
	// installations EvalSymlinks leaves a junction path unchanged, so compare
	// the directories' filesystem identity instead of their path strings.
	if !linkInfo.IsDir() || !releaseInfo.IsDir() || !os.SameFile(linkInfo, releaseInfo) {
		return fact
	}
	vpkPath := filepath.Join(metadata.ReleasePath, "pak01_dir.vpk")
	info, err := os.Stat(vpkPath)
	if err != nil || info.IsDir() {
		return fact
	}
	cache, ok := r.cache[vpkPath]
	digest := cache.digest
	if !ok || !os.SameFile(cache.info, info) || cache.info.Size() != info.Size() || !cache.info.ModTime().Equal(info.ModTime()) {
		file, err := os.Open(vpkPath)
		if err != nil {
			return fact
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		after, statErr := os.Stat(vpkPath)
		if copyErr != nil || closeErr != nil || statErr != nil || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
			return fact
		}
		digest = hex.EncodeToString(hash.Sum(nil))
		r.cache[vpkPath] = cachedVPK{info: after, digest: digest}
	}
	if digest != metadata.VPKSHA256 {
		return fact
	}
	fact.ContentVersionID = metadata.ContentVersionID
	fact.State = "confirmed"
	return fact
}
