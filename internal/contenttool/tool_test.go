package contenttool

import (
	"os"
	"path/filepath"
	"testing"

	controllerconfig "github.com/L4C99/dota2-arcade-platform/internal/controller/config"
)

func fixture(t *testing.T) (Config, string, string) {
	t.Helper()
	root := t.TempDir()
	content := filepath.Join(root, "content")
	dota := filepath.Join(root, "dota")
	for _, dir := range []string{content, filepath.Join(dota, "game", "dota_addons")} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatal(err)
		}
	}
	source1 := filepath.Join(root, "source-v1.vpk")
	source2 := filepath.Join(root, "source-v2.vpk")
	if err := os.WriteFile(source1, []byte("test immutable VPK one"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source2, []byte("test immutable VPK two"), 0640); err != nil {
		t.Fatal(err)
	}
	return Config{ContentRoot: content, DotaRoot: dota}, source1, source2
}

func TestPrepareSwitchRollbackAndImmutableVersion(t *testing.T) {
	c, v1, v2 := fixture(t)
	first, err := c.Prepare("123456", "v1", v1)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := c.Prepare("123456", "v1", v1); err != nil || again.VPKSHA256 != first.VPKSHA256 {
		t.Fatalf("idempotent prepare: %+v %v", again, err)
	}
	if _, err := c.Prepare("123456", "v1", v2); err == nil {
		t.Fatal("immutable release was overwritten")
	}
	if _, err := c.Prepare("../escape", "v2", v2); err == nil {
		t.Fatal("unsafe Workshop ID accepted")
	}
	if _, err := c.Prepare("123456", "../escape", v2); err == nil {
		t.Fatal("unsafe version accepted")
	}
	if _, err := c.Prepare("123456", "Pending", v2); err == nil {
		t.Fatal("transition metadata name accepted as version")
	}
	if _, err := c.Prepare("123456", "v3", filepath.Join(filepath.Dir(v1), "absent.vpk")); err == nil {
		t.Fatal("missing source accepted")
	}
	status, err := c.Status("123456")
	if err != nil || status.State != "not_switched" || len(status.PreparedVersions) != 1 {
		t.Fatalf("pre-switch status: %+v %v", status, err)
	}
	if err := c.Switch("123456", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Switch("123456", "v1"); err != nil {
		t.Fatalf("idempotent switch: %v", err)
	}
	second, err := c.Prepare("123456", "v2", v2)
	if err != nil {
		t.Fatal(err)
	}
	if second.VPKSHA256 == first.VPKSHA256 {
		t.Fatal("distinct VPK hash did not change")
	}
	if err := c.Switch("123456", "v2"); err != nil {
		t.Fatal(err)
	}
	status, err = c.Status("123456")
	if err != nil || status.CurrentVersion != "v2" || status.PreviousVersion != "v1" || status.State != "confirmed" {
		t.Fatalf("v2 status: %+v %v", status, err)
	}
	if err := c.Rollback("123456"); err != nil {
		t.Fatal(err)
	}
	if err := c.Rollback("123456"); err == nil {
		t.Fatal("second rollback should require an explicit prior switch")
	}
	status, err = c.Status("123456")
	if err != nil || status.CurrentVersion != "v1" || status.PreviousVersion != "" {
		t.Fatalf("rollback status: %+v %v", status, err)
	}
	if _, err := os.Stat(filepath.Join(c.ContentRoot, "123456", "releases", "v2", "pak01_dir.vpk")); err != nil {
		t.Fatalf("old release was removed: %v", err)
	}
}

func TestPrepareRepairsMatchingInterruptedMetadata(t *testing.T) {
	c, source, _ := fixture(t)
	base, release, _, _ := c.paths("123456", "v1")
	if err := os.MkdirAll(release, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "metadata"), 0750); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, "pak01_dir.vpk"), raw, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prepare("123456", "v1", source); err != nil {
		t.Fatalf("matching incomplete release was not repaired: %v", err)
	}
	if err := os.WriteFile(filepath.Join(release, "pak01_dir.vpk"), []byte("tampered"), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prepare("123456", "v1", source); err == nil {
		t.Fatal("tampered release was accepted")
	}
}

func TestSwitchRefusesUnknownAddonDirectory(t *testing.T) {
	c, source, _ := fixture(t)
	if _, err := c.Prepare("123456", "v1", source); err != nil {
		t.Fatal(err)
	}
	_, _, _, link := c.paths("123456", "")
	if err := os.Mkdir(link, 0750); err != nil {
		t.Fatal(err)
	}
	if err := c.Switch("123456", "v1"); err == nil {
		t.Fatal("unknown real addon directory was replaced")
	}
	if _, err := os.Stat(link); err != nil {
		t.Fatalf("unknown addon directory was removed: %v", err)
	}
}

func TestSwitchRecoversInterruptedLinkTransition(t *testing.T) {
	for _, phase := range []string{"old_moved", "new_installed"} {
		t.Run(phase, func(t *testing.T) {
			c, v1, v2 := fixture(t)
			if _, err := c.Prepare("123456", "v1", v1); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Prepare("123456", "v2", v2); err != nil {
				t.Fatal(err)
			}
			if err := c.Switch("123456", "v1"); err != nil {
				t.Fatal(err)
			}
			base, release, _, link := c.paths("123456", "v2")
			tempName, backupName := ".content-tool-123456-interrupted", ".content-tool-backup-123456-interrupted"
			temp, backup := filepath.Join(filepath.Dir(link), tempName), filepath.Join(filepath.Dir(link), backupName)
			if err := createDirectoryLink(temp, release); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(filepath.Join(base, "metadata", "pending.json"), transition{Old: "v1", New: "v2", TempName: tempName, BackupName: backupName}); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(link, backup); err != nil {
				t.Fatal(err)
			}
			if phase == "new_installed" {
				if err := os.Rename(temp, link); err != nil {
					t.Fatal(err)
				}
			}
			status, err := c.Status("123456")
			if err != nil || status.State != "transition_incomplete" {
				t.Fatalf("interrupted status: %+v %v", status, err)
			}
			if err := c.Switch("123456", "v2"); err != nil {
				t.Fatalf("recover and switch: %v", err)
			}
			status, err = c.Status("123456")
			if err != nil || status.State != "confirmed" || status.CurrentVersion != "v2" || status.PreviousVersion != "v1" {
				t.Fatalf("recovered status: %+v %v", status, err)
			}
			for _, path := range []string{temp, backup, filepath.Join(base, "metadata", "pending.json")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("stale transition path %s: %v", path, err)
				}
			}
		})
	}
}

func TestControllerReadsSwitchedMetadata(t *testing.T) {
	c, source, _ := fixture(t)
	if _, err := c.Prepare("123456", "v1", source); err != nil {
		t.Fatal(err)
	}
	if err := c.Switch("123456", "v1"); err != nil {
		t.Fatal(err)
	}
	base, _, _, link := c.paths("123456", "")
	config := controllerconfig.Config{ContentBindings: []controllerconfig.ContentBinding{{WorkshopID: "123456", CurrentLinkPath: link, MetadataPath: filepath.Join(base, "metadata", "current.json")}}}
	fact := controllerconfig.NewFactReader(config).Facts("test", 0).Content[0]
	if fact.State != "confirmed" || fact.ContentVersionID != "v1" || fact.VPKSHA256 == "" {
		t.Fatalf("Controller readback: %+v", fact)
	}
	if err := os.WriteFile(filepath.Join(base, "metadata", "current.json"), []byte(`{"version":"v1"}`), 0640); err != nil {
		t.Fatal(err)
	}
	fact = controllerconfig.NewFactReader(config).Facts("test", 0).Content[0]
	if fact.State != "unknown" {
		t.Fatalf("invalid current metadata accepted: %+v", fact)
	}
}

func TestOperationLockPreventsConcurrentMutation(t *testing.T) {
	c, source, _ := fixture(t)
	if _, err := c.Prepare("123456", "v1", source); err != nil {
		t.Fatal(err)
	}
	base, _, _, link := c.paths("123456", "")
	lock := filepath.Join(base, "metadata", ".content-tool.lock")
	if err := os.WriteFile(lock, []byte("another operator"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Switch("123456", "v1"); err == nil {
		t.Fatal("concurrent switch accepted")
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("link changed under operation lock: %v", err)
	}
}
