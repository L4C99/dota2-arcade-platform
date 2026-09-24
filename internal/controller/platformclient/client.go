package platformclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

type Client struct {
	baseURL string
	nodeID  string
	secret  string
	http    *http.Client
}

type StatusError struct {
	StatusCode int
	Message    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("platform API status %d: %s", e.StatusCode, e.Message)
}

func New(baseURL, nodeID, secret string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), nodeID: nodeID, secret: secret,
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

func (c *Client) Heartbeat(ctx context.Context, h nodev1.Heartbeat) (nodev1.HeartbeatResult, error) {
	var result nodev1.HeartbeatResult
	_, err := c.do(ctx, http.MethodPost, "/heartbeat", h, &result)
	return result, err
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
