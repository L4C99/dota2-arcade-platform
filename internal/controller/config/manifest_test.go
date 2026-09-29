package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateManifestDeterminismAndDrift(t *testing.T) {
	root := t.TempDir()
	template := filepath.Join(root, "template.json")
	depA, depB := filepath.Join(root, "a.cfg"), filepath.Join(root, "b.cfg")
	for path, value := range map[string]string{template: `{"arguments":["-test"]}`, depA: "a", depB: "bb"} {
		if err := os.WriteFile(path, []byte(value), 0444); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0444); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(root, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(root, 0755)
		for _, path := range []string{template, depA, depB} {
			_ = os.Chmod(path, 0644)
		}
	})
	c := Config{TemplateManifests: map[string]TemplateManifestBinding{"x": {VersionRoot: root, TemplatePath: template, Dependencies: []string{depA, depB}}}}
	_, first, err := c.TemplateManifestSHA256V1("x")
	if err != nil {
		t.Fatal(err)
	}
	c.TemplateManifests["x"] = TemplateManifestBinding{VersionRoot: root, TemplatePath: template, Dependencies: []string{depB, depA}}
	_, reordered, err := c.TemplateManifestSHA256V1("x")
	if err != nil || first != reordered {
		t.Fatalf("dependency order changed digest: %v", err)
	}
	c.TemplateManifests["x"] = TemplateManifestBinding{VersionRoot: root, TemplatePath: template, Dependencies: []string{depA, depA}}
	if _, _, err := c.TemplateManifestSHA256V1("x"); err == nil {
		t.Fatal("duplicate dependency accepted")
	}
	c.TemplateManifests["x"] = TemplateManifestBinding{VersionRoot: root, TemplatePath: template, Dependencies: []string{depA, depB}}
	if err := os.Chmod(depA, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(depA, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.TemplateManifestSHA256V1("x"); err == nil {
		t.Fatal("writable dependency accepted")
	}
	if err := os.Chmod(depA, 0444); err != nil {
		t.Fatal(err)
	}
	_, changed, err := c.TemplateManifestSHA256V1("x")
	if err != nil || changed == first {
		t.Fatalf("dependency drift not detected: %v", err)
	}
	if err := os.Chmod(template, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(template, []byte(`{"arguments":["-changed"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(template, 0444); err != nil {
		t.Fatal(err)
	}
	_, templateChanged, err := c.TemplateManifestSHA256V1("x")
	if err != nil || templateChanged == changed {
		t.Fatalf("template bytes did not change digest: %v", err)
	}
	depC := filepath.Join(root, "c.cfg")
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(depC, []byte("bb"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(depC, 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(depC, 0644) })
	c.TemplateManifests["x"] = TemplateManifestBinding{VersionRoot: root, TemplatePath: template, Dependencies: []string{depA, depC}}
	_, pathChanged, err := c.TemplateManifestSHA256V1("x")
	if err != nil || pathChanged == templateChanged {
		t.Fatalf("dependency path did not change digest: %v", err)
	}
	c.TemplateManifests["x"] = TemplateManifestBinding{VersionRoot: root, TemplatePath: "relative.json"}
	if _, _, err := c.TemplateManifestSHA256V1("x"); err == nil {
		t.Fatal("relative template accepted")
	}
	c.TemplateManifests["x"] = TemplateManifestBinding{VersionRoot: root, TemplatePath: template, Dependencies: []string{filepath.Join(root, "missing.cfg")}}
	if _, _, err := c.TemplateManifestSHA256V1("x"); err == nil {
		t.Fatal("missing dependency accepted")
	}
}
