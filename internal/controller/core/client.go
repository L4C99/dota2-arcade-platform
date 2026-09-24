// Package core adapts the fixed d2core v0.1.1 Go client without changing its
// process, port, readiness, recovery or reclaim semantics.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/L4C99/dota2-arcade-dedicated-core/client"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

type Caller interface {
	Call(context.Context, string, any) (json.RawMessage, error)
}

type Client struct{ caller Caller }

func New(dataDir string) (*Client, error) {
	c, err := client.New(dataDir)
	if err != nil {
		return nil, err
	}
	return &Client{caller: c}, nil
}

func NewWithCaller(c Caller) *Client { return &Client{caller: c} }

type CoreError = client.Error

type Accepted struct {
	Accepted    bool   `json:"accepted"`
	InstanceID  string `json:"instanceId"`
	OperationID string `json:"operationId"`
}

// API is the small subset of d2core used for P0 control operations.
type API interface {
	Create(context.Context, nodev1.FrozenCreate) (Accepted, error)
	Stop(context.Context, string) (Accepted, error)
	Operation(context.Context, string) (Operation, error)
	Status(context.Context, string) (Instance, error)
	List(context.Context) (ListResult, error)
}

type Operation struct {
	OperationID string     `json:"operationId"`
	Kind        string     `json:"kind"`
	InstanceID  string     `json:"instanceId"`
	Status      string     `json:"status"`
	Phase       string     `json:"phase"`
	Error       *CoreError `json:"error"`
}

type Instance struct {
	InstanceID         string     `json:"instanceId"`
	Port               int        `json:"port"`
	Lifecycle          string     `json:"lifecycle"`
	Process            string     `json:"process"`
	Room               string     `json:"room"`
	Cleanup            string     `json:"cleanup"`
	CurrentOperationID string     `json:"currentOperationId"`
	Error              *CoreError `json:"error"`
}

type ListResult struct {
	Instances []Instance `json:"instances"`
}

func (c *Client) call(ctx context.Context, method string, params any, output any) error {
	raw, err := c.caller.Call(ctx, method, params)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("decode core %s: %w", method, err)
	}
	return nil
}

func (c *Client) Create(ctx context.Context, frozen nodev1.FrozenCreate) (Accepted, error) {
	if frozen.IdempotencyKey == "" || frozen.Template == "" || frozen.Port < 0 || frozen.Port > 65535 {
		return Accepted{}, errors.New("incomplete frozen core create request")
	}
	params := struct {
		Template       string `json:"template"`
		Port           int    `json:"port"`
		IdempotencyKey string `json:"idempotencyKey"`
	}{frozen.Template, frozen.Port, frozen.IdempotencyKey}
	var accepted Accepted
	if err := c.call(ctx, "create", params, &accepted); err != nil {
		return Accepted{}, err
	}
	if !accepted.Accepted || accepted.InstanceID == "" || accepted.OperationID == "" {
		return Accepted{}, errors.New("incomplete d2core create acceptance; outcome unknown")
	}
	return accepted, nil
}

func (c *Client) Stop(ctx context.Context, instanceID string) (Accepted, error) {
	if instanceID == "" {
		return Accepted{}, errors.New("missing instance ID")
	}
	var accepted Accepted
	if err := c.call(ctx, "stop", map[string]string{"instanceId": instanceID}, &accepted); err != nil {
		return Accepted{}, err
	}
	if !accepted.Accepted || accepted.InstanceID == "" || accepted.OperationID == "" {
		return Accepted{}, errors.New("incomplete d2core stop acceptance; outcome unknown")
	}
	return accepted, nil
}

func (c *Client) Operation(ctx context.Context, operationID string) (Operation, error) {
	var op Operation
	if err := c.call(ctx, "operation", map[string]string{"operationId": operationID}, &op); err != nil {
		return Operation{}, err
	}
	if op.OperationID == "" || op.InstanceID == "" {
		return Operation{}, errors.New("incomplete d2core operation response")
	}
	return op, nil
}

func (c *Client) Status(ctx context.Context, instanceID string) (Instance, error) {
	var instance Instance
	if err := c.call(ctx, "status", map[string]string{"instanceId": instanceID}, &instance); err != nil {
		return Instance{}, err
	}
	if instance.InstanceID == "" {
		return Instance{}, errors.New("incomplete d2core status response")
	}
	return instance, nil
}

func (c *Client) List(ctx context.Context) (ListResult, error) {
	var result ListResult
	if err := c.call(ctx, "list", struct{}{}, &result); err != nil {
		return ListResult{}, err
	}
	if result.Instances == nil {
		result.Instances = []Instance{}
	}
	return result, nil
}

// ClearlyNoEffectCreateReject is deliberately narrow. A transport error or a
// structured error carrying core IDs is never treated as a safe rejection.
func ClearlyNoEffectCreateReject(err error) bool {
	var coreErr *client.Error
	if !errors.As(err, &coreErr) || coreErr.InstanceID != "" || coreErr.OperationID != "" {
		return false
	}
	if coreErr.Stage != "validate" && coreErr.Stage != "protocol" {
		return false
	}
	switch coreErr.Code {
	case "INVALID_REQUEST", "UNSUPPORTED_VERSION", "INVALID_TEMPLATE", "INVALID_PATH",
		"PORT_REQUIRED", "PORT_IN_USE", "NO_PORT_AVAILABLE", "INSUFFICIENT_STORAGE":
		return true
	default:
		return false
	}
}
