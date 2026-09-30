package deploy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestA4WindowsContentToolPreflight(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows installer")
	}
	root := t.TempDir()
	for _, dir := range []string{"bin", "config"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0750); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"node-controller.exe", "d2core.exe", "start-d2core-manager.ps1", "start-node-controller.ps1"} {
		if err := os.WriteFile(filepath.Join(root, "bin", name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "BUILD.json"), []byte(`{"version":"0.1.2","gitCommit":"6dddb5892f962e70beb32fc30df4a78bce595528"}`), 0600); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, "config", "secret")
	if err := os.WriteFile(secret, []byte("test-only"), 0600); err != nil {
		t.Fatal(err)
	}
	config := `{"nodeSecretFile":"` + strings.ReplaceAll(secret, `\`, `\\`) + `","network":{"localPortMin":28000,"localPortMax":28001}}`
	if err := os.WriteFile(filepath.Join(root, "config", "node-controller.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	run := func() ([]byte, error) {
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", "windows/install-node.ps1", "-Root", root, "-ValidateOnly").CombinedOutput()
	}
	if out, err := run(); err == nil || !strings.Contains(string(out), "content-tool.exe") {
		t.Fatalf("missing tool not rejected %s %v", out, err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "content-tool.exe"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := run(); err != nil || !strings.Contains(string(out), "preflight passed") {
		t.Fatalf("complete preflight %s %v", out, err)
	}

	for _, tc := range []struct{ name, manifest string }{
		{"old release", `{"version":"0.1.1","gitCommit":"988720ad85af1f0d97bfe98ec4da4fcbb070beea"}`},
		{"wrong commit", `{"version":"0.1.2","gitCommit":"988720ad85af1f0d97bfe98ec4da4fcbb070beea"}`},
		{"wrong version", `{"version":"0.1.3","gitCommit":"6dddb5892f962e70beb32fc30df4a78bce595528"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "bin", "BUILD.json"), []byte(tc.manifest), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := run(); err == nil || !strings.Contains(string(out), "d2core must be fixed v0.1.2") {
				t.Fatalf("incompatible build accepted: %s %v", out, err)
			}
		})
	}

}
func TestA4LinuxJQPreflight(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux wrapper")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, "systemd/start-d2core-manager.sh")
	cmd.Env = []string{"PATH=" + t.TempDir()}
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "required prerequisite missing: jq") {
		t.Fatalf("missing jq %s %v", out, err)
	}
}
