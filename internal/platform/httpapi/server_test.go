package httpapi

import "testing"

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
