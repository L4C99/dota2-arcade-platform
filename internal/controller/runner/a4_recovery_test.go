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
