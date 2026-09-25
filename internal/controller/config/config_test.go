package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/auth"
)

func validConfig(root string) Config {
	return Config{PlatformURL: "http://127.0.0.1:8080", Development: true,
		NodeID:          "11111111-2222-4333-8444-555555555555",
		NodeSecretFile:  filepath.Join(root, "node.secret"),
		D2CoreBuildFile: filepath.Join(root, "BUILD.json"), D2CoreDataDir: filepath.Join(root, "core-data"),
		HardMaxInstances: 1,
		Network:          nodev1.NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28000, MappingMode: "identity"},
		TemplateBindings: map[string]string{"n7-r1": filepath.Join(root, "template.json")}}
}

func TestConfigRequiresSafeTransportAndPaths(t *testing.T) {
	config := validConfig(t.TempDir())
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.PlatformURL = "http://node.example"
	if err := config.Validate(); err == nil {
		t.Fatal("non-loopback HTTP accepted")
	}
	config.PlatformURL = "https://platform.example"
	config.D2CoreDataDir = filepath.Join(t.TempDir(), "数据")
	if err := config.Validate(); err == nil {
		t.Fatal("non-ASCII d2core path accepted")
	}
}

func TestFactsReadback(t *testing.T) {
	root := t.TempDir()
	config := validConfig(root)
	manifest := []byte(`{"version":"0.1.1","gitCommit":"988720ad85af1f0d97bfe98ec4da4fcbb070beea"}`)
	if err := os.WriteFile(config.D2CoreBuildFile, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	config.ContentBindings = []ContentBinding{{WorkshopID: "123", CurrentLinkPath: filepath.Join(root, "current"), MetadataPath: filepath.Join(root, "current.json")}}
	h := config.Facts("test", 0)
	if h.D2CoreVersion != "0.1.1" || h.D2CoreCommit != nodev1.D2CoreCommit || h.Content[0].State != "unknown" {
		t.Fatalf("unexpected facts: %+v", h)
	}
	release := filepath.Join(root, "release")
	if err := os.Mkdir(release, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, "pak01_dir.vpk"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("test"))
	metadata, _ := json.Marshal(contentMetadata{WorkshopID: "123", ContentVersionID: "v1", ReleasePath: release, VPKSHA256: hex.EncodeToString(digest[:])})
	if err := os.WriteFile(config.ContentBindings[0].MetadataPath, metadata, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(release, config.ContentBindings[0].CurrentLinkPath); err != nil {
		if runtime.GOOS == "windows" {
			t.Logf("directory symlink unavailable in this Windows test context: %v", err)
			return
		}
		t.Fatal(err)
	}
	reader := NewFactReader(config)
	h = reader.Facts("test", 0)
	if h.Content[0].State != "confirmed" || h.Content[0].ContentVersionID != "v1" {
		t.Fatalf("content readback failed: %+v", h.Content[0])
	}
	if err := os.WriteFile(filepath.Join(release, "pak01_dir.vpk"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	h = reader.Facts("test", 0)
	if h.Content[0].State != "unknown" {
		t.Fatalf("changed VPK retained confirmed fact: %+v", h.Content[0])
	}
}

func TestLoadSecretFromSeparateFile(t *testing.T) {
	root := t.TempDir()
	config := validConfig(root)
	secret, _, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.NodeSecretFile, []byte(secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "controller.json")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, got, err := Load(path)
	if err != nil || got != secret || loaded.NodeID != config.NodeID {
		t.Fatalf("config load failed: %v", err)
	}
}
