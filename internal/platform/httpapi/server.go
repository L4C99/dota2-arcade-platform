package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
)

type Config struct {
	PublicOrigin string
	Development  bool
	WebRoot      string
}

func (c Config) Validate() error {
	u, err := url.Parse(c.PublicOrigin)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return errors.New("PLATFORM_PUBLIC_ORIGIN must be an origin without path or credentials")
	}
	if u.Scheme == "https" {
		return nil
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if c.Development && u.Scheme == "http" && (host == "localhost" || (ip != nil && ip.IsLoopback())) {
		return nil
	}
	return errors.New("non-HTTPS origin is allowed only for loopback development")
}

type api struct {
	store  *store.Store
	config Config
	gate   *loginGate
	leases *capabilityLeases
}

func NewHandler(s *store.Store, c Config) (http.Handler, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	a := &api{store: s, config: c, gate: newLoginGate(), leases: newCapabilityLeases()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/v1/session", a.createSession)
	mux.HandleFunc("GET /api/v1/me", a.me)
	mux.HandleFunc("GET /api/v1/catalog", a.playerCatalog)
	mux.HandleFunc("GET /api/v1/nodes", a.playerNodes)
	mux.HandleFunc("GET /api/v1/party", a.currentParty)
	mux.HandleFunc("POST /api/v1/party", a.createParty)
	mux.HandleFunc("GET /api/v1/party/invite", a.currentPartyInvite)
	mux.HandleFunc("POST /api/v1/party/invite/reset", a.resetPartyInvite)
	mux.HandleFunc("POST /api/v1/party/join", a.joinParty)
	mux.HandleFunc("POST /api/v1/party/leave", a.leaveParty)
	mux.HandleFunc("POST /api/v1/party/members/{id}/remove", a.removePartyMember)
	mux.HandleFunc("POST /api/v1/party/disband", a.disbandParty)
	mux.HandleFunc("POST /api/v1/server-requests", a.createPlayerRequest)
	mux.HandleFunc("GET /api/v1/server-requests/current", a.currentPlayerRequest)
	mux.HandleFunc("GET /api/v1/server-requests/{id}", a.playerRequest)
	mux.HandleFunc("POST /api/v1/server-requests/{id}/stop", a.stopPlayerRequest)
	mux.HandleFunc("POST /api/v1/server-requests/{id}/next-game", a.nextGamePlayerRequest)
	mux.HandleFunc("GET /api/v1/server-requests/{id}/next-game", a.playerNextGameIntent)
	mux.HandleFunc("POST /api/v1/server-requests/{id}/abandon", a.abandonPlayerRequest)
	mux.HandleFunc("POST /api/v1/server-requests/{id}/cancel", a.cancelPlayerRequest)
	mux.HandleFunc("GET /api/v1/server-requests/{id}/allocation", a.playerAllocation)
	mux.HandleFunc("POST /api/v1/session/logout", a.logout)
	mux.HandleFunc("POST /api/v1/admin/login", a.adminLogin)
	mux.HandleFunc("GET /api/v1/admin/me", a.adminMe)
	mux.HandleFunc("POST /api/v1/admin/logout", a.adminLogout)
	mux.HandleFunc("GET /api/v1/admin/overview", a.adminOverview)
	mux.HandleFunc("POST /api/v1/admin/actions", a.adminAction)
	mux.HandleFunc("POST "+nodev1.APIPath+"/heartbeat", a.nodeHeartbeat)
	mux.HandleFunc("POST "+nodev1.APIPath+"/session", a.nodeCapabilitySession)
	mux.HandleFunc("POST "+nodev1.APIPath+"/session/check", a.fencedNode(a.nodeCheckSession))
	mux.HandleFunc("POST "+nodev1.APIPath+"/inventory", a.fencedNode(a.nodeInventory))
	mux.HandleFunc("POST "+nodev1.APIPath+"/reconcile/complete", a.nodeReconcileComplete)
	mux.HandleFunc("GET "+nodev1.APIPath+"/allocations/active", a.fencedNode(a.nodeActiveAllocations))
	mux.HandleFunc("POST "+nodev1.APIPath+"/allocations/{id}/fact", a.fencedNode(a.nodeInstanceFact))
	mux.HandleFunc("GET "+nodev1.APIPath+"/jobs/open", a.fencedNode(a.nodeOpenJobs))
	mux.HandleFunc("POST "+nodev1.APIPath+"/jobs/claim", a.fencedNode(a.nodeClaimJob))
	mux.HandleFunc("GET "+nodev1.APIPath+"/jobs/{id}", a.fencedNode(a.nodeGetJob))
	mux.HandleFunc("POST "+nodev1.APIPath+"/jobs/{id}/prepare", a.fencedNode(a.nodePrepareJob))
	mux.HandleFunc("POST "+nodev1.APIPath+"/jobs/{id}/report", a.fencedNode(a.nodeReportJob))
	if c.WebRoot != "" {
		if !filepath.IsAbs(c.WebRoot) {
			return nil, errors.New("PLATFORM_WEB_ROOT must be absolute")
		}
		info, err := os.Stat(c.WebRoot)
		if err != nil || !info.IsDir() {
			return nil, errors.New("PLATFORM_WEB_ROOT must be an existing directory")
		}
		mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(c.WebRoot, "index.html"))
		})
		mux.Handle("GET /", http.FileServer(http.Dir(c.WebRoot)))
	}
	return mux, nil
}

func (a *api) secureCookie() bool { return strings.HasPrefix(a.config.PublicOrigin, "https://") }
func (a *api) userCookieName() string {
	if a.config.Development {
		return "arcade_session_dev"
	}
	return "__Host-arcade_session"
}
func (a *api) adminCookieName() string {
	if a.config.Development {
		return "arcade_admin_dev"
	}
	return "__Host-arcade_admin"
}

func (a *api) checkOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") != a.config.PublicOrigin {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return false
	}
	return true
}

func (a *api) setCookie(w http.ResponseWriter, name, token string, lifetime time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/", HttpOnly: true,
		Secure: a.secureCookie(), SameSite: http.SameSiteLaxMode,
		MaxAge: int(lifetime.Seconds())})
}

func (a *api) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: "/", HttpOnly: true,
		Secure: a.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Pool.Ping(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) writeUserIdentity(w http.ResponseWriter, r *http.Request, status int, id string) {
	name, err := a.store.UserDisplayName(r.Context(), id)
	if err != nil {
		http.Error(w, "session unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, status, map[string]string{"userId": id, "displayName": name})
}

func (a *api) createSession(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	if cookie, err := r.Cookie(a.userCookieName()); err == nil {
		if id, err := a.store.UserForToken(r.Context(), cookie.Value); err == nil {
			a.writeUserIdentity(w, r, http.StatusOK, id)
			return
		}
	}
	id, token, err := a.store.CreateUserSession(r.Context())
	if err != nil {
		http.Error(w, "session unavailable", http.StatusServiceUnavailable)
		return
	}
	a.setCookie(w, a.userCookieName(), token, 90*24*time.Hour)
	a.writeUserIdentity(w, r, http.StatusCreated, id)
}

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(a.userCookieName())
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := a.store.UserForToken(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	a.writeUserIdentity(w, r, http.StatusOK, id)
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	if cookie, err := r.Cookie(a.userCookieName()); err == nil {
		if err := a.store.RevokeUserToken(r.Context(), cookie.Value); err != nil && !errors.Is(err, store.ErrInvalidCredentials) {
			http.Error(w, "logout unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	a.clearCookie(w, a.userCookieName())
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, value any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return errors.New("expected JSON")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 16*1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func (a *api) adminLogin(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	remoteIP := loginClientIP(r, a.config.Development)
	key := remoteIP + ":" + strings.ToLower(strings.TrimSpace(request.Username))
	if !a.gate.Allowed(key) {
		http.Error(w, "login temporarily limited", http.StatusTooManyRequests)
		return
	}
	previousToken := ""
	if previous, err := r.Cookie(a.adminCookieName()); err == nil {
		previousToken = previous.Value
	}
	id, token, err := a.store.LoginAdminRotating(r.Context(), request.Username, request.Password, previousToken)
	if errors.Is(err, store.ErrInvalidCredentials) {
		a.gate.Failed(key)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "login unavailable", http.StatusServiceUnavailable)
		return
	}
	a.gate.Succeeded(key)
	a.setCookie(w, a.adminCookieName(), token, 12*time.Hour)
	writeJSON(w, http.StatusOK, map[string]string{"adminUserId": id})
}

func (a *api) adminMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(a.adminCookieName())
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := a.store.AdminForToken(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"adminUserId": id})
}

func (a *api) adminLogout(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	if cookie, err := r.Cookie(a.adminCookieName()); err == nil {
		if err := a.store.RevokeAdminToken(r.Context(), cookie.Value); err != nil && !errors.Is(err, store.ErrInvalidCredentials) {
			http.Error(w, "logout unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	a.clearCookie(w, a.adminCookieName())
	w.WriteHeader(http.StatusNoContent)
}

type loginGate struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}
type loginAttempt struct {
	failures int
	next     time.Time
	last     time.Time
}

func newLoginGate() *loginGate { return &loginGate{attempts: make(map[string]loginAttempt)} }

// Production listens on loopback behind Caddy, which overwrites this header.
// Public forwarding headers and non-loopback peers are never trusted.
func loginClientIP(r *http.Request, development bool) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if !development && ip != nil && ip.IsLoopback() {
		if values := r.Header.Values("X-Platform-Client-IP"); len(values) == 1 {
			if forwarded := net.ParseIP(values[0]); forwarded != nil {
				return forwarded.String()
			}
		}
	}
	if ip != nil {
		return ip.String()
	}
	return "invalid-peer"
}

func (g *loginGate) prune(now time.Time) {
	for key, attempt := range g.attempts {
		if now.Sub(attempt.last) > time.Hour {
			delete(g.attempts, key)
		}
	}
}
func (g *loginGate) Allowed(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if attempt, ok := g.attempts[key]; ok {
		return !now.Before(attempt.next)
	}
	if len(g.attempts) >= 10000 {
		g.prune(now)
	}
	if len(g.attempts) >= 10000 {
		return false
	}
	// Reserve a slot before password verification so concurrent novel keys
	// cannot overrun the bound or evict another client's active penalty.
	g.attempts[key] = loginAttempt{last: now}
	return true
}
func (g *loginGate) Failed(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if _, exists := g.attempts[key]; !exists && len(g.attempts) >= 10000 {
		return
	}
	a := g.attempts[key]
	if now.Sub(a.last) > time.Hour {
		a.failures = 0
	}
	a.failures++
	a.last = now
	if a.failures >= 3 {
		delay := time.Duration(1<<min(a.failures-3, 6)) * time.Second
		a.next = now.Add(delay)
	}
	g.attempts[key] = a
}
func (g *loginGate) Succeeded(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attempts, key)
}
