package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/L4C99/dota2-arcade-dedicated-core/client"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

type fakeCaller struct {
	method string
	params any
	result string
	err    error
}

func (f *fakeCaller) Call(_ context.Context, method string, params any) (json.RawMessage, error) {
	f.method, f.params = method, params
	return json.RawMessage(f.result), f.err
}

func TestCreateSendsOnlyFrozenParameters(t *testing.T) {
	f := &fakeCaller{result: `{"accepted":true,"instanceId":"i_1","operationId":"o_1"}`}
	c := NewWithCaller(f)
	_, err := c.Create(context.Background(), nodev1.FrozenCreate{IdempotencyKey: "nodejob-abc", Template: "C:/core/template.json", Port: 28000})
	if err != nil || f.method != "create" {
		t.Fatalf("create: %v %s", err, f.method)
	}
	raw, _ := json.Marshal(f.params)
	if string(raw) != `{"template":"C:/core/template.json","port":28000,"idempotencyKey":"nodejob-abc"}` {
		t.Fatalf("unexpected request: %s", raw)
	}
}

func TestNoEffectRejectionIsNarrow(t *testing.T) {
	cases := []struct {
		err  error
		safe bool
	}{
		{&client.Error{Code: "INVALID_TEMPLATE", Stage: "validate"}, true},
		{&client.Error{Code: "PORT_IN_USE", Stage: "validate", InstanceID: "i_1"}, false},
		{&client.Error{Code: "IO_ERROR", Stage: "persist"}, false},
		{&client.Error{Code: "IDEMPOTENCY_CONFLICT", Stage: "validate"}, false},
		{errors.New("timeout"), false},
	}
	for _, tc := range cases {
		if got := ClearlyNoEffectCreateReject(tc.err); got != tc.safe {
			t.Fatalf("%v: got %t", tc.err, got)
		}
	}
}
