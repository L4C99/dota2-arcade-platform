// Package nodev1 is the versioned Platform Server / Node Controller contract.
package nodev1

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
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
	Content               []ContentFact `json:"content"`
}

type HeartbeatResult struct {
	CompatibilityStatus string `json:"compatibilityStatus"`
	ReportedAt          string `json:"reportedAt"`
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
	State       string `json:"state"`
	InstanceID  string `json:"instanceId,omitempty"`
	OperationID string `json:"operationId,omitempty"`
	ErrorCode   string `json:"errorCode,omitempty"`
	ErrorStage  string `json:"errorStage,omitempty"`
}

var workshopIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
var domainPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)
var coreTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

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
	}
	return nil
}

func (n NetworkFacts) Validate(hardMax int) error {
	if n.ConnectHost == "" || len(n.ConnectHost) > 253 ||
		(net.ParseIP(n.ConnectHost) == nil && (!domainPattern.MatchString(n.ConnectHost) || strings.Contains(n.ConnectHost, ".."))) {
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
