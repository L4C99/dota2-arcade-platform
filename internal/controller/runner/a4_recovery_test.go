package runner

import (
	"context"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
	"testing"
	"time"
)

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
