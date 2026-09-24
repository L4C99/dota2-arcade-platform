package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/auth"
)

type ContentBinding struct {
	WorkshopID      string `json:"workshopId"`
	CurrentLinkPath string `json:"currentLinkPath"`
	MetadataPath    string `json:"metadataPath"`
}

type Config struct {
	PlatformURL      string              `json:"platformUrl"`
	Development      bool                `json:"development"`
	NodeID           string              `json:"nodeId"`
	NodeSecretFile   string              `json:"nodeSecretFile"`
	D2CoreBuildFile  string              `json:"d2coreBuildFile"`
	D2CoreDataDir    string              `json:"d2coreDataDir"`
	HardMaxInstances int                 `json:"hardMaxInstances"`
	Network          nodev1.NetworkFacts `json:"network"`
	TemplateBindings map[string]string   `json:"templateBindings"`
	ContentBindings  []ContentBinding    `json:"contentBindings"`
}

var nodeIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var workshopIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

func Load(path string) (Config, string, error) {
	if !filepath.IsAbs(path) {
		return Config{}, "", errors.New("controller config path must be absolute")
	}
	file, err := os.Open(path)
	if err != nil {
		return Config{}, "", err
	}
	defer file.Close()
	var c Config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return Config{}, "", err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, "", errors.New("trailing config JSON")
	}
	if err := c.Validate(); err != nil {
		return Config{}, "", err
	}
	raw, err := os.ReadFile(c.NodeSecretFile)
	if err != nil {
		return Config{}, "", err
	}
	secret := strings.TrimSpace(string(raw))
	if _, ok := auth.TokenHash(secret); !ok {
		return Config{}, "", errors.New("invalid node secret file")
	}
	return c, secret, nil
}

func (c Config) Validate() error {
	u, err := url.Parse(c.PlatformURL)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("platformUrl must be a URL origin")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(c.Development && u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))) {
		return errors.New("controller requires HTTPS except for loopback development")
	}
	if !nodeIDPattern.MatchString(c.NodeID) || !filepath.IsAbs(c.NodeSecretFile) {
		return errors.New("node ID and absolute secret file are required")
	}
	for _, path := range []string{c.D2CoreBuildFile, c.D2CoreDataDir} {
		if err := validateCorePath(path); err != nil {
			return err
		}
	}
	if err := c.Network.Validate(c.HardMaxInstances); err != nil {
		return err
	}
	for key, path := range c.TemplateBindings {
		if key == "" {
			return errors.New("empty template binding name")
		}
		if err := validateCorePath(path); err != nil {
			return fmt.Errorf("template binding %q: %w", key, err)
		}
	}
	seen := make(map[string]bool)
	for _, binding := range c.ContentBindings {
		if !workshopIDPattern.MatchString(binding.WorkshopID) || seen[binding.WorkshopID] || !filepath.IsAbs(binding.CurrentLinkPath) || !filepath.IsAbs(binding.MetadataPath) {
			return errors.New("invalid content binding")
		}
		seen[binding.WorkshopID] = true
	}
	return nil
}

func validateCorePath(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("d2core path must be absolute")
	}
	for _, r := range path {
		if r > 127 || r < 32 {
			return errors.New("d2core path must contain printable ASCII only")
		}
	}
	return nil
}
