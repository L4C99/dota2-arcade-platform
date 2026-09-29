package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

const capabilityLeaseDuration = 90 * time.Second

// A lease belongs to one Controller process and one Node. Replacing it
// invalidates the previous process, and a Platform restart invalidates all
// leases. Only token digests are retained; no lease is written to a log or DB.
type capabilityLease struct {
	digest       [32]byte
	processID    string
	capabilities map[string]bool
	expires      time.Time
}

type capabilityLeases struct {
	mu     sync.RWMutex
	byNode map[string]capabilityLease
}

func newCapabilityLeases() *capabilityLeases {
	return &capabilityLeases{byNode: make(map[string]capabilityLease)}
}

func (l *capabilityLeases) issue(nodeID string, input nodev1.CapabilitySessionRequest) (nodev1.CapabilitySession, bool) {
	if len(input.ProcessID) < 16 || len(input.ProcessID) > 128 || nodev1.ValidateCapabilities(input.Capabilities) != nil {
		return nodev1.CapabilitySession{}, false
	}
	capabilities := make(map[string]bool, len(input.Capabilities))
	for _, value := range input.Capabilities {
		capabilities[value] = true
	}
	if !capabilities[nodev1.CapabilityContentValidationV102] || !capabilities[nodev1.CapabilityTemplateManifestV1] || !capabilities[nodev1.CapabilityCoreInventoryV1] {
		return nodev1.CapabilitySession{}, false
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nodev1.CapabilitySession{}, false
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	lease := capabilityLease{digest: sha256.Sum256([]byte(token)), processID: input.ProcessID, capabilities: capabilities, expires: time.Now().UTC().Add(capabilityLeaseDuration)}
	l.mu.Lock()
	l.byNode[nodeID] = lease
	l.mu.Unlock()
	return nodev1.CapabilitySession{Token: token, ExpiresAt: lease.expires.Format(time.RFC3339Nano)}, true
}

func (l *capabilityLeases) valid(nodeID, token, capability string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.validLocked(nodeID, token, capability)
}

func (l *capabilityLeases) validLocked(nodeID, token, capability string) bool {
	if token == "" || len(token) > 128 {
		return false
	}
	digest := sha256.Sum256([]byte(token))
	lease, ok := l.byNode[nodeID]
	return ok && time.Now().UTC().Before(lease.expires) && lease.capabilities[capability] && subtle.ConstantTimeCompare(digest[:], lease.digest[:]) == 1
}

type leaseFenceContextKey struct{}

// Session replacement waits for already authorized requests to finish. Once
// the replacement response is sent, no request using the prior token can
// still mutate an execution.
func (a *api) fencedNode(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.leases.mu.RLock()
		defer a.leases.mu.RUnlock()
		next(w, r.WithContext(context.WithValue(r.Context(), leaseFenceContextKey{}, true)))
	}
}

func (a *api) requestHasNodeCapability(r *http.Request, nodeID, capability string) bool {
	if r.Context().Value(leaseFenceContextKey{}) == true {
		return a.leases.validLocked(nodeID, r.Header.Get("X-Controller-Session"), capability)
	}
	return a.leases.valid(nodeID, r.Header.Get("X-Controller-Session"), capability)
}

func (a *api) nodeCapabilitySession(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	var input nodev1.CapabilitySessionRequest
	if decodeNodeJSON(w, r, &input) != nil {
		http.Error(w, "invalid session request", http.StatusBadRequest)
		return
	}
	session, ok := a.leases.issue(nodeID, input)
	if !ok {
		http.Error(w, "invalid session capability", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, session)
}

func (a *api) nodeCheckSession(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	if !a.requestHasNodeCapability(r, nodeID, nodev1.CapabilityContentValidationV102) {
		http.Error(w, "capability required", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) requestHasCapability(r *http.Request, nodeID, capability string) bool {
	if capability == "" || capability == "legacy_v1" {
		return true
	}
	if capability != nodev1.RequiredContentValidationV102 {
		return false
	}
	return a.requestHasNodeCapability(r, nodeID, nodev1.CapabilityContentValidationV102)
}

func (a *api) requestCapability(r *http.Request, nodeID string) string {
	if a.requestHasCapability(r, nodeID, nodev1.RequiredContentValidationV102) {
		return nodev1.RequiredContentValidationV102
	}
	return "legacy_v1"
}
