package platformclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNodeSecretNotForwardedAcrossRedirect(t *testing.T) {
	received := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := New(source.URL, "node-id", "test-secret")
	_, err := client.OpenJobs(context.Background())
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("redirect was not rejected: %v", err)
	}
	if received {
		t.Fatal("redirect target received the Node request")
	}
}
