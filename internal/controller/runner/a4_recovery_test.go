package runner

import (
	"context"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
	"testing"
	"time"
)

func TestA4RepeatedEmptyInventoryAndRetryRejectionsKeepUnknown(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	p.Prepare(context.Background(), p.job.ID, nodev1.PrepareCreateRequest{Template: testTemplate(t), Port: 28000})
	p.job.State = "unknown"
	frozen := *p.job.FrozenCreate
	for _, code := range []string{"NO_PORT_AVAILABLE", "PORT_IN_USE", "NO_PORT_AVAILABLE", "INVALID_REQUEST"} {
		// New Runner/Core objects also model restart. Neither an empty inventory
		// nor unchanged local paths/markers are consumed as continuity proof.
		c := &fakeCore{err: &core.CoreError{Code: code, Stage: "validate"}}
		r := Runner{Platform: p, Core: c}
		if err := r.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
		if p.job.State != "unknown" || len(c.keys) != 1 || c.keys[0] != frozen {
			t.Fatalf("unsafe recovery: %+v", p)
		}
	}
}

func TestA4TerminalCreateJoinRepairIsReachable(t *testing.T) {
	p := &reconcilePlatform{allocations: []nodev1.ActiveAllocation{{ID: "a", InstanceID: "i", State: "running"}}}
	c := &fakeCore{instance: core.Instance{InstanceID: "i", Lifecycle: "active", Process: "running", Room: "ready", Port: 28000}}
	r := Runner{Platform: p, Core: c, Network: nodev1.NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28001, LocalPortMax: 28001, MappingMode: "identity"}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.facts[0].JoinInfoErrorCode == "" {
		t.Fatal("missing mapping not reported")
	}
	r.Network.LocalPortMin = 28000
	r.Network.LocalPortMax = 28000
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.facts[1].JoinInfo == nil || c.creates != 0 || c.stops != 0 {
		t.Fatal("terminal create could not repair JoinInfo")
	}
}

type a4StopPlatform struct {
	fakePlatform
	stop                 nodev1.Job
	ordinary, restricted int
}

func (p *a4StopPlatform) Claim(context.Context) (*nodev1.Job, error) { p.ordinary++; return nil, nil }
func (p *a4StopPlatform) ClaimIndependentStop(context.Context) (*nodev1.Job, error) {
	p.restricted++
	return &p.stop, nil
}
func (p *a4StopPlatform) Report(_ context.Context, _ string, r nodev1.ReportRequest) (nodev1.Job, error) {
	p.reports = append(p.reports, r)
	return p.stop, nil
}

func TestA4UnknownAllowsOnlyIndependentStop(t *testing.T) {
	p := &a4StopPlatform{fakePlatform: fakePlatform{job: testJob()}, stop: nodev1.Job{ID: "stop", Kind: "stop", State: "claimed", InstanceID: "known"}}
	p.Prepare(context.Background(), p.job.ID, nodev1.PrepareCreateRequest{Template: testTemplate(t), Port: 28000})
	p.job.State = "unknown"
	p.job.PreparedAtUnix = time.Now().Add(-40 * 24 * time.Hour).Unix()
	c := &fakeCore{result: core.Accepted{InstanceID: "known", OperationID: "stop-op"}, op: core.Operation{OperationID: "stop-op", InstanceID: "known", Kind: "stop", Status: "succeeded"}, instance: core.Instance{InstanceID: "known", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.ordinary != 0 || p.restricted != 1 || c.creates != 0 || p.job.State != "unknown" || len(p.reports) != 2 || p.reports[1].State != "succeeded" {
		t.Fatalf("unsafe flow: %+v %+v", p, c)
	}
}

func TestA4SuccessfulCreateObservesCurrentInstance(t *testing.T) {
	for _, tc := range []struct{ lifecycle, process, room, want, code string }{
		{"failed", "stopped", "failed", "failed_with_effect", "INSTANCE_FAILED"},
		{"active", "running", "loading", "accepted", ""},
		{"failed", "unknown", "failed", "failed_with_effect", "IDENTITY_UNVERIFIED"},
		{"reclaimed", "stopped", "", "failed_with_effect", "INSTANCE_FAILED"},
	} {
		t.Run(tc.lifecycle+tc.process+tc.room, func(t *testing.T) {
			p := &fakePlatform{job: testJob()}
			p.job.State = "accepted"
			p.job.InstanceID = "i"
			p.job.OperationID = "o"
			c := &fakeCore{op: core.Operation{OperationID: "o", InstanceID: "i", Kind: "create", Status: "succeeded"}, instance: core.Instance{InstanceID: "i", Lifecycle: tc.lifecycle, Process: tc.process, Room: tc.room}}
			r := Runner{Platform: p, Core: c}
			if err := r.observe(context.Background(), p.job); err != nil {
				t.Fatal(err)
			}
			if p.job.State != tc.want {
				t.Fatalf("state %s want %s", p.job.State, tc.want)
			}
			if tc.code != "" && p.reports[0].ErrorCode != tc.code {
				t.Fatalf("reports %+v", p.reports)
			}
			if c.stops != 0 || c.creates != 0 {
				t.Fatal("observation caused direct side effects")
			}
		})
	}
}

func TestA4StopRetriesAfterOldWorkerTeardown(t *testing.T) {
	p := &fakePlatform{job: nodev1.Job{ID: "new-stop", Kind: "stop", State: "claimed", InstanceID: "i"}}
	c := &fakeCore{result: core.Accepted{InstanceID: "i", OperationID: "old"}, op: core.Operation{OperationID: "old", InstanceID: "i", Kind: "stop", Status: "failed"}, instance: core.Instance{InstanceID: "i", CurrentOperationID: "old", Lifecycle: "failed", Process: "stopped", Cleanup: "failed"}}
	r := Runner{Platform: p, Core: c}
	for i := 0; i < 2; i++ {
		if err := r.stop(context.Background(), p.job); err != nil {
			t.Fatal(err)
		}
		if p.job.State != "unknown" || p.job.OperationID != "" {
			t.Fatalf("adopted old worker: %+v", p.job)
		}
	}
	c.instance.CurrentOperationID = ""
	c.instance.Cleanup = "complete"
	c.instance.Lifecycle = "reclaimed"
	c.result.OperationID = "new"
	c.op.OperationID = "new"
	c.op.Status = "succeeded"
	if err := r.stop(context.Background(), p.job); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "succeeded" || p.job.OperationID != "new" || c.stops != 3 {
		t.Fatalf("retry failed: %+v %+v", p, c)
	}
}

func TestA4RunningStopAdoptedAndOwnFailureTerminal(t *testing.T) {
	p := &fakePlatform{job: nodev1.Job{ID: "stop", Kind: "stop", State: "unknown", InstanceID: "i"}}
	c := &fakeCore{op: core.Operation{OperationID: "running", InstanceID: "i", Kind: "stop", Status: "running"}, instance: core.Instance{InstanceID: "i", CurrentOperationID: "running", Lifecycle: "active", Process: "running"}}
	r := Runner{Platform: p, Core: c}
	if err := r.stop(context.Background(), p.job); err != nil {
		t.Fatal(err)
	}
	if c.stops != 0 || p.job.OperationID != "running" {
		t.Fatal("duplicated running stop")
	}
	c.op.Status = "failed"
	c.instance.Cleanup = "failed"
	if err := r.stop(context.Background(), p.job); err != nil {
		t.Fatal(err)
	}
	if c.stops != 0 || p.job.State != "failed_with_effect" || p.job.OperationID != "running" {
		t.Fatal("own operation identity changed")
	}
}
