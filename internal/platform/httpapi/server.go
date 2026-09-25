package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
)

type Config struct {
	PublicOrigin string
	Development  bool
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
}

func NewHandler(s *store.Store, c Config) (http.Handler, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	a := &api{store: s, config: c, gate: newLoginGate()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/v1/session", a.createSession)
	mux.HandleFunc("GET /api/v1/me", a.me)
	mux.HandleFunc("GET /api/v1/catalog", a.playerCatalog)
	mux.HandleFunc("POST /api/v1/server-requests", a.createPlayerRequest)
	mux.HandleFunc("GET /api/v1/server-requests/current", a.currentPlayerRequest)
	mux.HandleFunc("GET /api/v1/server-requests/{id}", a.playerRequest)
	mux.HandleFunc("POST /api/v1/server-requests/{id}/stop", a.stopPlayerRequest)
	mux.HandleFunc("GET /api/v1/server-requests/{id}/allocation", a.playerAllocation)
	mux.HandleFunc("POST /api/v1/session/logout", a.logout)
	mux.HandleFunc("POST /api/v1/admin/login", a.adminLogin)
	mux.HandleFunc("GET /api/v1/admin/me", a.adminMe)
	mux.HandleFunc("POST /api/v1/admin/logout", a.adminLogout)
	mux.HandleFunc("POST "+nodev1.APIPath+"/heartbeat", a.nodeHeartbeat)
	mux.HandleFunc("GET "+nodev1.APIPath+"/jobs/open", a.nodeOpenJobs)
	mux.HandleFunc("POST "+nodev1.APIPath+"/jobs/claim", a.nodeClaimJob)
	mux.HandleFunc("GET "+nodev1.APIPath+"/jobs/{id}", a.nodeGetJob)
	mux.HandleFunc("POST "+nodev1.APIPath+"/jobs/{id}/prepare", a.nodePrepareJob)
	mux.HandleFunc("POST "+nodev1.APIPath+"/jobs/{id}/report", a.nodeReportJob)
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

func (a *api) createSession(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	if cookie, err := r.Cookie(a.userCookieName()); err == nil {
		if id, err := a.store.UserForToken(r.Context(), cookie.Value); err == nil {
			writeJSON(w, http.StatusOK, map[string]string{"userId": id})
			return
		}
	}
	id, token, err := a.store.CreateUserSession(r.Context())
	if err != nil {
		http.Error(w, "session unavailable", http.StatusServiceUnavailable)
		return
	}
	a.setCookie(w, a.userCookieName(), token, 90*24*time.Hour)
	writeJSON(w, http.StatusCreated, map[string]string{"userId": id})
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
	writeJSON(w, http.StatusOK, map[string]string{"userId": id})
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	if cookie, err := r.Cookie(a.userCookieName()); err == nil {
		_ = a.store.RevokeUserToken(r.Context(), cookie.Value)
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
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}
	key := remoteIP + ":" + strings.ToLower(strings.TrimSpace(request.Username))
	if !a.gate.Allowed(key) {
		http.Error(w, "login temporarily limited", http.StatusTooManyRequests)
		return
	}
	id, token, err := a.store.LoginAdmin(r.Context(), request.Username, request.Password)
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
		_ = a.store.RevokeAdminToken(r.Context(), cookie.Value)
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
func (g *loginGate) Allowed(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !time.Now().Before(g.attempts[key].next)
}
func (g *loginGate) Failed(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if len(g.attempts) > 10000 {
		g.attempts = make(map[string]loginAttempt)
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
