package platformclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

type Client struct {
	baseURL        string
	nodeID         string
	secret         string
	http           *http.Client
	mu             sync.Mutex
	renewMu        sync.Mutex
	processID      string
	sessionToken   string
	sessionExpires time.Time
}

type StatusError struct {
	StatusCode int
	Message    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("platform API status %d: %s", e.StatusCode, e.Message)
}

func New(baseURL, nodeID, secret string) *Client {
	var nonce [16]byte
	processID := ""
	if _, err := rand.Read(nonce[:]); err == nil {
		processID = hex.EncodeToString(nonce[:])
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), nodeID: nodeID, secret: secret, processID: processID,
		http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) do(ctx context.Context, method, path string, input, output any) (int, error) {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+nodev1.APIPath+path, body)
	if err != nil {
		return 0, err
	}
	request.Header.Set("X-Node-ID", c.nodeID)
	request.Header.Set("Authorization", "Bearer "+c.secret)
	c.mu.Lock()
	token := c.sessionToken
	c.mu.Unlock()
	if token != "" {
		request.Header.Set("X-Controller-Session", token)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return response.StatusCode, &StatusError{response.StatusCode, strings.TrimSpace(string(data))}
	}
	if response.StatusCode == http.StatusNoContent || output == nil {
		return response.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(output); err != nil {
		return response.StatusCode, err
	}
	return response.StatusCode, nil
}

// EnsureSession renews a short-lived, process-local capability lease. The
// Node credential alone never authorizes v1.0.2 execution routes.
func (c *Client) EnsureSession(ctx context.Context) error {
	c.renewMu.Lock()
	defer c.renewMu.Unlock()
	c.mu.Lock()
	if c.sessionToken != "" && time.Until(c.sessionExpires) > 45*time.Second {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	if c.processID == "" {
		return fmt.Errorf("cannot generate Controller process identity")
	}
	var session nodev1.CapabilitySession
	_, err := c.do(ctx, http.MethodPost, "/session", nodev1.CapabilitySessionRequest{ProcessID: c.processID,
		Capabilities: []string{nodev1.CapabilityContentValidationV102, nodev1.CapabilityTemplateManifestV1, nodev1.CapabilityCoreInventoryV1}}, &session)
	if err != nil {
		return err
	}
	expires, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	if err != nil || session.Token == "" || time.Until(expires) < 30*time.Second {
		return fmt.Errorf("invalid capability session response")
	}
	c.mu.Lock()
	c.sessionToken, c.sessionExpires = session.Token, expires
	c.mu.Unlock()
	return nil
}

func (c *Client) ReportInventory(ctx context.Context, report nodev1.InventoryReport) error {
	_, err := c.do(ctx, http.MethodPost, "/inventory", report, nil)
	return err
}

func (c *Client) DropSession() {
	c.mu.Lock()
	c.sessionToken, c.sessionExpires = "", time.Time{}
	c.mu.Unlock()
}

func (c *Client) CheckSession(ctx context.Context) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	_, err := c.do(ctx, http.MethodPost, "/session/check", nil, nil)
	return err
}

func (c *Client) BeginOperation(ctx context.Context, jobID string, input nodev1.OperationStartRequest) (nodev1.Job, error) {
	var job nodev1.Job
	_, err := c.do(ctx, http.MethodPost, "/jobs/"+jobID+"/operation-start", input, &job)
	return job, err
}

func (c *Client) Heartbeat(ctx context.Context, h nodev1.Heartbeat) (nodev1.HeartbeatResult, error) {
	var result nodev1.HeartbeatResult
	_, err := c.do(ctx, http.MethodPost, "/heartbeat", h, &result)
	return result, err
}

func (c *Client) CompleteReconcile(ctx context.Context, generation int64) error {
	_, err := c.do(ctx, http.MethodPost, "/reconcile/complete", struct {
		Generation int64 `json:"generation"`
	}{generation}, nil)
	return err
}

func (c *Client) ActiveAllocations(ctx context.Context) ([]nodev1.ActiveAllocation, error) {
	var allocations []nodev1.ActiveAllocation
	_, err := c.do(ctx, http.MethodGet, "/allocations/active", nil, &allocations)
	return allocations, err
}

func (c *Client) ReportInstanceFact(ctx context.Context, id string, fact nodev1.InstanceFact) error {
	_, err := c.do(ctx, http.MethodPost, "/allocations/"+id+"/fact", fact, nil)
	return err
}

func (c *Client) OpenJobs(ctx context.Context) ([]nodev1.Job, error) {
	var jobs []nodev1.Job
	_, err := c.do(ctx, http.MethodGet, "/jobs/open", nil, &jobs)
	return jobs, err
}

func (c *Client) Claim(ctx context.Context) (*nodev1.Job, error) {
	var job nodev1.Job
	status, err := c.do(ctx, http.MethodPost, "/jobs/claim", nil, &job)
	if err != nil || status == http.StatusNoContent {
		return nil, err
	}
	return &job, nil
}

func (c *Client) ClaimIndependentStop(ctx context.Context) (*nodev1.Job, error) {
	var job nodev1.Job
	status, err := c.do(ctx, http.MethodPost, "/jobs/claim?independentStop=true", nil, &job)
	if err != nil || status == http.StatusNoContent {
		return nil, err
	}
	return &job, nil
}

func (c *Client) GetJob(ctx context.Context, jobID string) (nodev1.Job, error) {
	var job nodev1.Job
	_, err := c.do(ctx, http.MethodGet, "/jobs/"+jobID, nil, &job)
	return job, err
}

func (c *Client) Prepare(ctx context.Context, jobID string, input nodev1.PrepareCreateRequest) (nodev1.FrozenCreate, error) {
	var result nodev1.FrozenCreate
	_, err := c.do(ctx, http.MethodPost, "/jobs/"+jobID+"/prepare", input, &result)
	return result, err
}

func (c *Client) Report(ctx context.Context, jobID string, input nodev1.ReportRequest) (nodev1.Job, error) {
	var job nodev1.Job
	_, err := c.do(ctx, http.MethodPost, "/jobs/"+jobID+"/report", input, &job)
	return job, err
}
