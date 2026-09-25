package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOriginConfig(t *testing.T) {
	cases := []struct {
		config Config
		valid  bool
	}{
		{Config{PublicOrigin: "https://example.org"}, true},
		{Config{PublicOrigin: "http://127.0.0.1:8080", Development: true}, true},
		{Config{PublicOrigin: "http://example.org", Development: true}, false},
		{Config{PublicOrigin: "http://127.0.0.1:8080"}, false},
		{Config{PublicOrigin: "https://example.org/path"}, false},
		{Config{PublicOrigin: ""}, false},
	}
	for _, tc := range cases {
		if got := tc.config.Validate() == nil; got != tc.valid {
			t.Errorf("origin %q development=%t accepted=%t", tc.config.PublicOrigin, tc.config.Development, got)
		}
	}
}

func TestPlayerWebStaticRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("player web"), 0644); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(nil, Config{PublicOrigin: "https://example.org", WebRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "player web") {
		t.Fatalf("player page code=%d body=%q", w.Code, w.Body.String())
	}
	if _, err := NewHandler(nil, Config{PublicOrigin: "https://example.org", WebRoot: "relative"}); err == nil {
		t.Fatal("relative static path accepted")
	}
}
