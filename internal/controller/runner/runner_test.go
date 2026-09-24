package runner

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	coreclient "github.com/L4C99/dota2-arcade-dedicated-core/client"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

type fakePlatform struct {
	job      nodev1.Job
	prepared bool
	reports  []nodev1.ReportRequest
}

func (p *fakePlatform) OpenJobs(context.Context) ([]nodev1.Job, error) {
	if p.job.State == "succeeded" {
		return nil, nil
	}
	return []nodev1.Job{p.job}, nil
}
func (p *fakePlatform) Claim(context.Context) (*nodev1.Job, error) {
	p.job.State = "claimed"
	j := p.job
	return &j, nil
}
func (p *fakePlatform) Prepare(_ context.Context, _ string, in nodev1.PrepareCreateRequest) (nodev1.FrozenCreate, error) {
	p.prepared = true
	key := "nodejob-" + "123456781234123412341234567890ab"
	digest := nodev1.CreateFingerprint(key, in.Template, in.Port)
	f := nodev1.FrozenCreate{IdempotencyKey: key, Template: in.Template, Port: in.Port, FingerprintSHA256: hex.EncodeToString(digest[:])}
	p.job.FrozenCreate = &f
	return f, nil
}
func (p *fakePlatform) Report(_ context.Context, _ string, r nodev1.ReportRequest) (nodev1.Job, error) {
	p.reports = append(p.reports, r)
	p.job.State = r.State
	if r.InstanceID != "" {
		p.job.InstanceID = r.InstanceID
	}
	if r.OperationID != "" {
		p.job.OperationID = r.OperationID
	}
	return p.job, nil
}

type fakeCore struct {
	creates  int
	stops    int
	result   core.Accepted
	err      error
	op       core.Operation
	instance core.Instance
}

func (c *fakeCore) Create(context.Context, nodev1.FrozenCreate) (core.Accepted, error) {
	c.creates++
	return c.result, c.err
}
func (c *fakeCore) Stop(context.Context, string) (core.Accepted, error) {
	c.stops++
	return c.result, c.err
}
func (c *fakeCore) Operation(context.Context, string) (core.Operation, error) { return c.op, nil }
func (c *fakeCore) Status(context.Context, string) (core.Instance, error)     { return c.instance, nil }
func (c *fakeCore) List(context.Context) (core.ListResult, error)             { return core.ListResult{}, nil }

func testJob() nodev1.Job {
	return nodev1.Job{ID: "12345678-1234-1234-1234-1234567890ab", Kind: "create", State: "pending", TemplateBindingKey: "test", RequestedPort: 28000}
}

func TestCreatePreparedAcceptedThenReady(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"}, op: core.Operation{OperationID: "o_1", Kind: "create", InstanceID: "i_1", Status: "succeeded"}, instance: core.Instance{InstanceID: "i_1", Lifecycle: "active", Process: "running", Room: "ready"}}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": "C:/core/template.json"}, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !p.prepared || c.creates != 1 || p.job.State != "succeeded" || len(p.reports) != 2 || p.reports[0].State != "accepted" {
		t.Fatalf("flow: %+v %+v", p, c)
	}
}

func TestExistingClaimedJobNeverBlindlyReplaysCreate(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	p.job.State = "claimed"
	c := &fakeCore{}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": "C:/core/template.json"}, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 0 || p.job.State != "unknown" {
		t.Fatalf("blind replay: %+v %+v", p, c)
	}
}

func TestTimeoutStaysUnknown(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	c := &fakeCore{err: errors.New("response lost")}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": "C:/core/template.json"}, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 1 || p.job.State != "unknown" {
		t.Fatalf("lost response: %+v %+v", p, c)
	}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 1 {
		t.Fatal("replayed unknown create")
	}
}

func TestIdempotencyConflictCannotReleaseJob(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	c := &fakeCore{err: &coreclient.Error{Code: "IDEMPOTENCY_CONFLICT", Stage: "validate"}}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": "C:/core/template.json"}, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "unknown" {
		t.Fatalf("conflicting key released as no-effect: %+v", p.job)
	}
}

func TestStopRequiresFullReclaim(t *testing.T) {
	p := &fakePlatform{job: nodev1.Job{ID: "12345678-1234-1234-1234-1234567890ab", Kind: "stop", State: "pending", InstanceID: "i_1"}}
	c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"}, op: core.Operation{OperationID: "o_1", Kind: "stop", InstanceID: "i_1", Status: "succeeded"}, instance: core.Instance{InstanceID: "i_1", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "pending"}}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "accepted" || c.stops != 1 {
		t.Fatalf("premature stop success: %+v %+v", p, c)
	}
	c.instance.Cleanup = "complete"
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "succeeded" || c.stops != 1 {
		t.Fatalf("reclaim did not converge: %+v %+v", p, c)
	}
}
