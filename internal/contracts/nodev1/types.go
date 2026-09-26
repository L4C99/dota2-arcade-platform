// Package nodev1 is the versioned Platform Server / Node Controller contract.
package nodev1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

const (
	APIPath               = "/api/node/v1"
	APIVersion            = 1
	D2CoreVersion         = "0.1.1"
	D2CoreCommit          = "988720ad85af1f0d97bfe98ec4da4fcbb070beea"
	D2CoreProtocolVersion = 1
)

type PortMapping struct {
	Local  int `json:"local"`
	Public int `json:"public"`
}

type NetworkFacts struct {
	ConnectHost  string        `json:"connectHost"`
	ProtocolIP   string        `json:"protocolIp"`
	LocalPortMin int           `json:"localPortMin"`
	LocalPortMax int           `json:"localPortMax"`
	MappingMode  string        `json:"mappingMode"`
	Mappings     []PortMapping `json:"mappings"`
	A2SEnabled   bool          `json:"a2sEnabled"`
}

type ContentFact struct {
	WorkshopID       string `json:"workshopId"`
	ContentVersionID string `json:"contentVersionId,omitempty"`
	VPKSHA256        string `json:"vpkSha256,omitempty"`
	State            string `json:"state"`
}

type Heartbeat struct {
	OS                    string        `json:"os"`
	ControllerVersion     string        `json:"controllerVersion"`
	NodeAPIVersion        int           `json:"nodeApiVersion"`
	D2CoreVersion         string        `json:"d2coreVersion"`
	D2CoreCommit          string        `json:"d2coreCommit"`
	D2CoreProtocolVersion int           `json:"d2coreProtocolVersion"`
	HardMaxInstances      int           `json:"hardMaxInstances"`
	Network               NetworkFacts  `json:"network"`
	A2SQueryOK            bool          `json:"a2sQueryOk"`
	Content               []ContentFact `json:"content"`
}

type HeartbeatResult struct {
	CompatibilityStatus          string `json:"compatibilityStatus"`
	ReportedAt                   string `json:"reportedAt"`
	ReconcileRequestedGeneration int64  `json:"reconcileRequestedGeneration"`
	ReconcileCompletedGeneration int64  `json:"reconcileCompletedGeneration"`
}

type ActiveAllocation struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	State      string `json:"state"`
	HasOpenJob bool   `json:"hasOpenJob"`
}

type InstanceFact struct {
	InstanceID string `json:"instanceId"`
	Outcome    string `json:"outcome"`
	Lifecycle  string `json:"lifecycle"`
	Process    string `json:"process"`
	Cleanup    string `json:"cleanup"`
	Port       int    `json:"port"`
}

type Job struct {
	ID                 string        `json:"id"`
	Kind               string        `json:"kind"`
	State              string        `json:"state"`
	IntegrationOnly    bool          `json:"integrationOnly"`
	TemplateBindingKey string        `json:"templateBindingKey,omitempty"`
	RequestedPort      int           `json:"requestedPort"`
	InstanceID         string        `json:"instanceId,omitempty"`
	OperationID        string        `json:"operationId,omitempty"`
	FrozenCreate       *FrozenCreate `json:"frozenCreate,omitempty"`
	PreparedAtUnix     int64         `json:"preparedAtUnix,omitempty"`
}

// CreateFingerprint is the Platform's frozen request digest. d2core applies
// its own normalized template-path/port fingerprint under the same stable key.
func CreateFingerprint(key, template string, port int) [32]byte {
	request, _ := json.Marshal(struct {
		IdempotencyKey string `json:"idempotencyKey"`
		Template       string `json:"template"`
		Port           int    `json:"port"`
	}{key, template, port})
	return sha256.Sum256(request)
}

type FrozenCreate struct {
	IdempotencyKey    string `json:"idempotencyKey"`
	Template          string `json:"template"`
	Port              int    `json:"port"`
	FingerprintSHA256 string `json:"fingerprintSha256"`
}

type PrepareCreateRequest struct {
	Template string `json:"template"`
	Port     int    `json:"port"`
}

type ReportRequest struct {
	State             string    `json:"state"`
	InstanceID        string    `json:"instanceId,omitempty"`
	OperationID       string    `json:"operationId,omitempty"`
	ErrorCode         string    `json:"errorCode,omitempty"`
	ErrorStage        string    `json:"errorStage,omitempty"`
	JoinInfo          *JoinInfo `json:"joinInfo,omitempty"`
	JoinInfoErrorCode string    `json:"joinInfoErrorCode,omitempty"`
}

type JoinInfo struct {
	LocalPort           int    `json:"localPort"`
	PublicPort          int    `json:"publicPort"`
	ConnectHost         string `json:"connectHost"`
	ProtocolIP          string `json:"protocolIp,omitempty"`
	EntryConfigRevision string `json:"entryConfigRevision"`
}

var workshopIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
var coreTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var revisionPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validConnectHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !asciiAlnum(label[0]) || !asciiAlnum(label[len(label)-1]) {
			return false
		}
		for i := 1; i < len(label)-1; i++ {
			if !asciiAlnum(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}

func asciiAlnum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// EntryConfigRevision covers the complete potential port set and entry
// configuration. Mapping order in a local JSON file does not affect it.
func EntryConfigRevision(n NetworkFacts) string {
	mappings := append([]PortMapping(nil), n.Mappings...)
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].Local < mappings[j].Local })
	encoded, _ := json.Marshal(struct {
		ConnectHost  string        `json:"connectHost"`
		ProtocolIP   string        `json:"protocolIp"`
		LocalPortMin int           `json:"localPortMin"`
		LocalPortMax int           `json:"localPortMax"`
		MappingMode  string        `json:"mappingMode"`
		Mappings     []PortMapping `json:"mappings"`
		A2SEnabled   bool          `json:"a2sEnabled"`
	}{n.ConnectHost, n.ProtocolIP, n.LocalPortMin, n.LocalPortMax, n.MappingMode, mappings, n.A2SEnabled})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// PublicPorts is the complete set of possible game-port entry targets for the
// current node configuration. It is used to require full human URI coverage.
func PublicPorts(n NetworkFacts) []int {
	ports := make([]int, 0)
	if n.MappingMode == "identity" {
		for p := n.LocalPortMin; p <= n.LocalPortMax && p <= 65535; p++ {
			ports = append(ports, p)
		}
	} else {
		for _, mapping := range n.Mappings {
			ports = append(ports, mapping.Public)
		}
	}
	sort.Ints(ports)
	return ports
}

func (j JoinInfo) Validate() error {
	if j.LocalPort < 1 || j.LocalPort > 65535 || j.PublicPort < 1 || j.PublicPort > 65535 ||
		!validConnectHost(j.ConnectHost) ||
		(j.ProtocolIP != "" && net.ParseIP(j.ProtocolIP) == nil) || !revisionPattern.MatchString(j.EntryConfigRevision) {
		return fmt.Errorf("invalid JoinInfo")
	}
	return nil
}

func (r ReportRequest) Validate() error {
	switch r.State {
	case "accepted", "unknown", "succeeded", "rejected_no_effect", "failed_with_effect":
	default:
		return fmt.Errorf("invalid NodeJob report state")
	}
	for _, value := range []string{r.InstanceID, r.OperationID, r.ErrorCode, r.ErrorStage} {
		if value != "" && !coreTokenPattern.MatchString(value) {
			return fmt.Errorf("invalid NodeJob report field")
		}
	}
	if r.JoinInfoErrorCode != "" && !coreTokenPattern.MatchString(r.JoinInfoErrorCode) {
		return fmt.Errorf("invalid JoinInfo error")
	}
	if r.JoinInfo != nil {
		if r.State != "succeeded" || r.JoinInfoErrorCode != "" {
			return fmt.Errorf("unexpected JoinInfo")
		}
		if err := r.JoinInfo.Validate(); err != nil {
			return err
		}
	}
	if r.JoinInfoErrorCode != "" && r.State != "succeeded" {
		return fmt.Errorf("unexpected JoinInfo error")
	}
	return nil
}

func (h Heartbeat) Validate() error {
	if h.OS != "windows" && h.OS != "linux" {
		return fmt.Errorf("invalid OS")
	}
	if strings.TrimSpace(h.ControllerVersion) == "" || len(h.ControllerVersion) > 128 {
		return fmt.Errorf("invalid controller version")
	}
	if h.NodeAPIVersion < 0 {
		return fmt.Errorf("invalid node API version")
	}
	if len(h.D2CoreVersion) > 128 || len(h.D2CoreCommit) > 128 {
		return fmt.Errorf("invalid core version")
	}
	if h.HardMaxInstances < 0 {
		return fmt.Errorf("invalid hard max")
	}
	if err := h.Network.Validate(h.HardMaxInstances); err != nil {
		return err
	}
	if h.A2SQueryOK && !h.Network.A2SEnabled {
		return fmt.Errorf("A2S query cannot be OK when disabled")
	}
	if len(h.Content) > 128 {
		return fmt.Errorf("too many content facts")
	}
	seen := make(map[string]bool)
	for _, item := range h.Content {
		if !workshopIDPattern.MatchString(item.WorkshopID) || seen[item.WorkshopID] {
			return fmt.Errorf("invalid or duplicate workshop ID")
		}
		seen[item.WorkshopID] = true
		if item.State != "confirmed" && item.State != "unknown" {
			return fmt.Errorf("invalid content state")
		}
		if item.State == "confirmed" && item.ContentVersionID == "" {
			return fmt.Errorf("confirmed content has no version")
		}
		if len(item.ContentVersionID) > 128 || strings.ContainsAny(item.ContentVersionID, "\r\n\x00") {
			return fmt.Errorf("invalid content version")
		}
		if item.VPKSHA256 != "" && (item.State != "confirmed" || !revisionPattern.MatchString(item.VPKSHA256)) {
			return fmt.Errorf("invalid content digest fact")
		}
	}
	return nil
}

func (n NetworkFacts) Validate(hardMax int) error {
	if hardMax < 0 {
		return fmt.Errorf("invalid hard max")
	}
	if !validConnectHost(n.ConnectHost) {
		return fmt.Errorf("invalid connect host")
	}
	if n.ProtocolIP != "" && net.ParseIP(n.ProtocolIP) == nil {
		return fmt.Errorf("invalid protocol IP")
	}
	if n.LocalPortMin < 1 || n.LocalPortMax > 65535 || n.LocalPortMin > n.LocalPortMax {
		return fmt.Errorf("invalid local port range")
	}
	count := n.LocalPortMax - n.LocalPortMin + 1
	if count > 4096 || hardMax > count {
		return fmt.Errorf("hard max exceeds port pool")
	}
	if n.MappingMode != "identity" && n.MappingMode != "explicit" {
		return fmt.Errorf("invalid mapping mode")
	}
	if n.MappingMode == "identity" {
		if len(n.Mappings) != 0 {
			return fmt.Errorf("identity mapping must not supply explicit mappings")
		}
		return nil
	}
	if len(n.Mappings) != count {
		return fmt.Errorf("incomplete explicit mapping")
	}
	localSeen := make(map[int]bool)
	publicSeen := make(map[int]bool)
	for _, mapping := range n.Mappings {
		if mapping.Local < n.LocalPortMin || mapping.Local > n.LocalPortMax || mapping.Public < 1 || mapping.Public > 65535 || localSeen[mapping.Local] || publicSeen[mapping.Public] {
			return fmt.Errorf("invalid or duplicate mapping")
		}
		localSeen[mapping.Local] = true
		publicSeen[mapping.Public] = true
	}
	return nil
}

func Compatible(h Heartbeat) bool {
	return h.NodeAPIVersion == APIVersion &&
		(h.D2CoreVersion == D2CoreVersion || h.D2CoreVersion == "v"+D2CoreVersion) &&
		h.D2CoreCommit == D2CoreCommit && h.D2CoreProtocolVersion == D2CoreProtocolVersion
}
