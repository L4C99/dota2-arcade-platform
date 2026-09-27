package httpapi

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestA4LoginTrustedProxy(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "192.0.2.99")
	r.Header.Set("X-Platform-Client-IP", "2001:db8:0:0::1")
	if got := loginClientIP(r, false); got != "2001:db8::1" {
		t.Fatal(got)
	}
	if got := loginClientIP(r, true); got != "127.0.0.1" {
		t.Fatal(got)
	}
	r.RemoteAddr = "192.0.2.1:2345"
	if got := loginClientIP(r, false); got != "192.0.2.1" {
		t.Fatal(got)
	}
	r.RemoteAddr = "127.0.0.1:2345"
	r.Header.Set("X-Platform-Client-IP", "192.0.2.1, 192.0.2.2")
	if got := loginClientIP(r, false); got != "127.0.0.1" {
		t.Fatal(got)
	}
}

func TestA4LoginFloodCannotErasePenalty(t *testing.T) {
	g := newLoginGate()
	for i := 0; i < 9; i++ {
		g.Failed("victim")
	}
	for i := 0; i < 20000; i++ {
		key := fmt.Sprint(i)
		if g.Allowed(key) {
			g.Failed(key)
		}
	}
	if g.Allowed("victim") || len(g.attempts) > 10000 || g.Allowed("overflow") {
		t.Fatal("flood erased penalty or exceeded capacity")
	}
	g.attempts["0"] = loginAttempt{last: time.Now().Add(-2 * time.Hour)}
	if !g.Allowed("new-client") {
		t.Fatal("expired slot was not reclaimed")
	}
	if g.Allowed("victim") {
		t.Fatal("prune erased active penalty")
	}
}
