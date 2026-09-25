package runner

import (
	"context"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

type reconcilePlatform struct {
	fakePlatform
	allocations []nodev1.ActiveAllocation
	facts       []nodev1.InstanceFact
}

func (p *reconcilePlatform) OpenJobs(context.Context) ([]nodev1.Job, error) { return nil, nil }
func (p *reconcilePlatform) Claim(context.Context) (*nodev1.Job, error)     { return nil, nil }
func (p *reconcilePlatform) ActiveAllocations(context.Context) ([]nodev1.ActiveAllocation, error) {
	return p.allocations, nil
}
func (p *reconcilePlatform) ReportInstanceFact(_ context.Context, _ string, f nodev1.InstanceFact) error {
	p.facts = append(p.facts, f)
	return nil
}

func TestActiveReconcileNeverCreatesAndRequiresFullReclaim(t *testing.T) {
	p := &reconcilePlatform{allocations: []nodev1.ActiveAllocation{{ID: "allocation", InstanceID: "i_existing", State: "running"}}}
	c := &fakeCore{instance: core.Instance{InstanceID: "i_existing", Port: 28000, Lifecycle: "active", Process: "running", Cleanup: "pending"}}
	r := &Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 0 || c.stops != 0 || len(p.facts) != 1 || p.facts[0].Outcome != "active" || p.facts[0].Port != 28000 {
		t.Fatalf("reconcile active: creates=%d stops=%d facts=%+v", c.creates, c.stops, p.facts)
	}
	c.instance = core.Instance{InstanceID: "i_existing", Port: 28000, Lifecycle: "reclaimed", Process: "stopped", Cleanup: "failed"}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.facts[1].Outcome != "uncertain" {
		t.Fatalf("incomplete cleanup released: %+v", p.facts[1])
	}
	c.instance.Cleanup = "complete"
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.facts[2].Outcome != "reclaimed" || c.creates != 0 || c.stops != 0 {
		t.Fatalf("full reclaim: %+v", p.facts)
	}
}
