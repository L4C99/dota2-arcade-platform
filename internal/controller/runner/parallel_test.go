package runner

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

// These fakes keep the same durable job and core identities across Runner
// objects. They deliberately do not infer an unknown create's outcome.
type parallelPlatform struct {
	mu          sync.Mutex
	jobs        []nodev1.Job
	allocations map[string]string
	prepared    map[string]nodev1.FrozenCreate
	reports     map[string][]nodev1.ReportRequest
}

func newParallelPlatform() *parallelPlatform {
	return &parallelPlatform{allocations: map[string]string{}, prepared: map[string]nodev1.FrozenCreate{}, reports: map[string][]nodev1.ReportRequest{}}
}

func (p *parallelPlatform) add(i int, kind, allocation, instance string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
	p.jobs = append(p.jobs, nodev1.Job{ID: id, Kind: kind, State: "pending", InstanceID: instance,
		TemplateBindingKey: "test", RequestedPort: 0})
	p.allocations[id] = allocation
	return id
}

func (p *parallelPlatform) job(id string) nodev1.Job {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, job := range p.jobs {
		if job.ID == id {
			return job
		}
	}
	return nodev1.Job{}
}

func (p *parallelPlatform) OpenJobs(context.Context) ([]nodev1.Job, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var open []nodev1.Job
	for _, job := range p.jobs {
		if job.State == "pending" || job.State == "claimed" || job.State == "accepted" || job.State == "unknown" {
			open = append(open, job)
		}
	}
	return open, nil
}

func (p *parallelPlatform) Claim(context.Context) (*nodev1.Job, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.jobs {
		candidate := &p.jobs[i]
		if candidate.State != "pending" {
			continue
		}
		conflict := false
		for _, other := range p.jobs {
			if other.ID == candidate.ID || other.State != "claimed" && other.State != "accepted" && other.State != "unknown" {
				continue
			}
			if p.allocations[candidate.ID] != "" && p.allocations[candidate.ID] == p.allocations[other.ID] ||
				candidate.InstanceID != "" && candidate.InstanceID == other.InstanceID {
				conflict = true
				break
			}
		}
		if !conflict {
			candidate.State = "claimed"
			copy := *candidate
			return &copy, nil
		}
	}
	return nil, nil
}

func (p *parallelPlatform) Prepare(_ context.Context, id string, in nodev1.PrepareCreateRequest) (nodev1.FrozenCreate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := "nodejob-" + strings.ReplaceAll(id, "-", "")
	digest := nodev1.CreateFingerprint(key, in.Template, in.Port)
	frozen := nodev1.FrozenCreate{IdempotencyKey: key, Template: in.Template, Port: in.Port, FingerprintSHA256: hex.EncodeToString(digest[:])}
	if old, ok := p.prepared[id]; ok && old != frozen {
		return nodev1.FrozenCreate{}, errors.New("frozen create changed")
	}
	p.prepared[id] = frozen
	for i := range p.jobs {
		if p.jobs[i].ID == id {
			p.jobs[i].FrozenCreate = &frozen
			p.jobs[i].PreparedAtUnix = time.Now().Unix()
			break
		}
	}
	return frozen, nil
}

func (p *parallelPlatform) Report(_ context.Context, id string, report nodev1.ReportRequest) (nodev1.Job, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.jobs {
		if p.jobs[i].ID == id {
			j := &p.jobs[i]
			j.State = report.State
			if report.InstanceID != "" {
				j.InstanceID = report.InstanceID
			}
			if report.OperationID != "" {
				j.OperationID = report.OperationID
			}
			p.reports[id] = append(p.reports[id], report)
			return *j, nil
		}
	}
	return nodev1.Job{}, errors.New("missing job")
}

type parallelCore struct {
	mu           sync.Mutex
	mode         map[string]string
	creates      map[string]int
	stops        map[string]int
	operations   map[string]core.Operation
	instances    map[string]core.Instance
	operationErr map[string]error
}

type barrierCore struct {
	*parallelCore
	entered chan string
	release <-chan struct{}
}

type blockingOperationCore struct {
	*parallelCore
	blockedID string
	entered   chan struct{}
	release   <-chan struct{}
}

func (c *blockingOperationCore) Operation(ctx context.Context, id string) (core.Operation, error) {
	if id == c.blockedID {
		c.entered <- struct{}{}
		select {
		case <-c.release:
		case <-ctx.Done():
			return core.Operation{}, ctx.Err()
		}
	}
	return c.parallelCore.Operation(ctx, id)
}

func (c *barrierCore) Create(ctx context.Context, f nodev1.FrozenCreate) (core.Accepted, error) {
	c.entered <- f.IdempotencyKey
	select {
	case <-c.release:
	case <-ctx.Done():
		return core.Accepted{}, ctx.Err()
	}
	return c.parallelCore.Create(ctx, f)
}

func newParallelCore() *parallelCore {
	return &parallelCore{mode: map[string]string{}, creates: map[string]int{}, stops: map[string]int{},
		operations: map[string]core.Operation{}, instances: map[string]core.Instance{}, operationErr: map[string]error{}}
}

func (c *parallelCore) Create(_ context.Context, f nodev1.FrozenCreate) (core.Accepted, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.creates[f.IdempotencyKey]++
	if c.mode[f.IdempotencyKey] == "unknown" {
		return core.Accepted{}, errors.New("lost create response")
	}
	instance, operation := "i_"+f.IdempotencyKey, "o_"+f.IdempotencyKey
	status := "running"
	if c.mode[f.IdempotencyKey] == "failed" {
		status = "failed"
	}
	c.operations[operation] = core.Operation{OperationID: operation, InstanceID: instance, Kind: "create", Status: status}
	c.instances[instance] = core.Instance{InstanceID: instance, Port: 28000, Lifecycle: "active", Process: "running", Room: "loading"}
	return core.Accepted{Accepted: true, InstanceID: instance, OperationID: operation}, nil
}

func (c *parallelCore) Stop(_ context.Context, instance string) (core.Accepted, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stops[instance]++
	op := "stop_" + instance
	c.operations[op] = core.Operation{OperationID: op, InstanceID: instance, Kind: "stop", Status: "succeeded"}
	c.instances[instance] = core.Instance{InstanceID: instance, Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete", CurrentOperationID: op}
	return core.Accepted{Accepted: true, InstanceID: instance, OperationID: op}, nil
}

func (c *parallelCore) Operation(_ context.Context, id string) (core.Operation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.operationErr[id]; err != nil {
		return core.Operation{}, err
	}
	return c.operations[id], nil
}

func (c *parallelCore) Status(_ context.Context, id string) (core.Instance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.instances[id], nil
}

func (c *parallelCore) List(context.Context) (core.ListResult, error) {
	var result core.ListResult
	result.Storage.HistoryDays = 30
	return result, nil
}

func (c *parallelCore) ready(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	op := c.operations[id]
	op.Status = "succeeded"
	c.operations[id] = op
	instance := c.instances[op.InstanceID]
	instance.Room = "ready"
	c.instances[op.InstanceID] = instance
}

func parallelRunner(t *testing.T, p *parallelPlatform, c *parallelCore, limit int) Runner {
	t.Helper()
	return Runner{Platform: p, Core: c, MaxConcurrentJobs: limit, TemplateBindings: map[string]string{"test": testTemplate(t)},
		Network: nodev1.NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28000, MappingMode: "identity"}}
}

func TestIndependentCreatesAdvanceWhileFirstAccepted(t *testing.T) {
	p, c := newParallelPlatform(), newParallelCore()
	a := p.add(1, "create", "allocation-a", "")
	r := parallelRunner(t, p, c, 2)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(a).State != "accepted" {
		t.Fatal("A did not remain in running operation")
	}
	b := p.add(2, "create", "allocation-b", "")
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(a).State != "accepted" || p.job(b).State != "accepted" || c.creates["nodejob-"+strings.ReplaceAll(b, "-", "")] != 1 {
		t.Fatalf("B waited for A terminal: A=%+v B=%+v", p.job(a), p.job(b))
	}
	c.ready(p.job(b).OperationID)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(a).State != "accepted" || p.job(b).State != "succeeded" {
		t.Fatal("slow A blocked B Ready")
	}
}

func TestTwoIndependentCoreCreateCallsOverlap(t *testing.T) {
	p := newParallelPlatform()
	p.add(1, "create", "allocation-a", "")
	p.add(2, "create", "allocation-b", "")
	release := make(chan struct{})
	c := &barrierCore{parallelCore: newParallelCore(), entered: make(chan string, 2), release: release}
	r := parallelRunner(t, p, c.parallelCore, 2)
	r.Core = c
	done := make(chan error, 1)
	go func() { done <- r.Step(context.Background()) }()
	for i := 0; i < 2; i++ {
		select {
		case <-c.entered:
		case <-time.After(3 * time.Second):
			close(release)
			<-done
			t.Fatal("independent core create calls did not overlap")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if p.job("00000000-0000-0000-0000-000000000001").State != "accepted" ||
		p.job("00000000-0000-0000-0000-000000000002").State != "accepted" {
		t.Fatal("overlapping calls did not persist separate accepted jobs")
	}
}

func TestBlockedAObservationDoesNotBlockBCreate(t *testing.T) {
	p, c := newParallelPlatform(), newParallelCore()
	a := p.add(1, "create", "allocation-a", "")
	r := parallelRunner(t, p, c, 2)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	b := p.add(2, "create", "allocation-b", "")
	release := make(chan struct{})
	blocked := &blockingOperationCore{parallelCore: c, blockedID: p.job(a).OperationID,
		entered: make(chan struct{}, 1), release: release}
	r.Core = blocked
	done := make(chan error, 1)
	go func() { done <- r.Step(context.Background()) }()
	select {
	case <-blocked.entered:
	case <-time.After(3 * time.Second):
		close(release)
		<-done
		t.Fatal("A observation did not start")
	}
	deadline := time.After(3 * time.Second)
	for p.job(b).State != "accepted" {
		select {
		case <-deadline:
			close(release)
			<-done
			t.Fatal("B create waited for blocked A observation")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(release)
	if err := <-done; err != nil || p.job(a).State != "accepted" {
		t.Fatalf("blocked A recovery changed: %v %+v", err, p.job(a))
	}
}

func TestUnknownIndependentCreateAndFrozenRetry(t *testing.T) {
	p, c := newParallelPlatform(), newParallelCore()
	a := p.add(1, "create", "allocation-a", "")
	keyA := "nodejob-" + strings.ReplaceAll(a, "-", "")
	c.mode[keyA] = "unknown"
	r := parallelRunner(t, p, c, 2)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	frozen := p.prepared[a]
	r.TemplateBindings["test"] = testTemplate(t) + ".changed"
	b := p.add(2, "create", "allocation-b", "")
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(a).State != "unknown" || p.job(b).State != "accepted" || p.prepared[a] != frozen || c.creates[keyA] != 2 {
		t.Fatalf("unknown/frozen regression: A=%+v B=%+v", p.job(a), p.job(b))
	}
	if p.job(a).InstanceID != "" || p.job(a).OperationID != "" {
		t.Fatal("unknown acquired another job's identity")
	}
}

func TestRestartRecoversTwoAcceptedWithoutCreate(t *testing.T) {
	p, c := newParallelPlatform(), newParallelCore()
	a := p.add(1, "create", "allocation-a", "")
	b := p.add(2, "create", "allocation-b", "")
	r := parallelRunner(t, p, c, 2)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	ja, jb := p.job(a), p.job(b)
	if ja.State != "accepted" || jb.State != "accepted" || ja.InstanceID == jb.InstanceID || ja.OperationID == jb.OperationID {
		t.Fatalf("independent identities: A=%+v B=%+v", ja, jb)
	}
	c.ready(jb.OperationID)
	r = parallelRunner(t, p, c, 2) // new Controller process, same durable facts
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(a).State != "accepted" || p.job(b).State != "succeeded" || c.creates[ja.FrozenCreate.IdempotencyKey] != 1 || c.creates[jb.FrozenCreate.IdempotencyKey] != 1 {
		t.Fatal("restart duplicated or crossed create identity")
	}
}

func TestSameAllocationCreateConflictAndIndependentStop(t *testing.T) {
	p, c := newParallelPlatform(), newParallelCore()
	a := p.add(1, "create", "allocation-a", "")
	duplicate := p.add(2, "create", "allocation-a", "")
	r := parallelRunner(t, p, c, 3)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(a).State != "accepted" || p.job(duplicate).State != "pending" || len(c.creates) != 1 {
		t.Fatal("same Allocation duplicate create was dispatched")
	}
	// Another already running instance can stop while A is still loading.
	instance := "i_existing"
	c.instances[instance] = core.Instance{InstanceID: instance, Lifecycle: "active", Process: "running"}
	stop := p.add(3, "stop", "allocation-b", instance)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(stop).State != "succeeded" || p.job(a).State != "accepted" || c.stops[instance] != 1 ||
		c.instances[instance].Cleanup != "complete" || c.instances[instance].Lifecycle != "reclaimed" {
		t.Fatal("independent stop/full reclaim regressed")
	}
}

func TestTwoIndependentStopsRequireFullReclaim(t *testing.T) {
	p, c := newParallelPlatform(), newParallelCore()
	a := p.add(1, "create", "allocation-a", "")
	b := p.add(2, "create", "allocation-b", "")
	r := parallelRunner(t, p, c, 2)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	ja, jb := p.job(a), p.job(b)
	c.ready(ja.OperationID)
	c.ready(jb.OperationID)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(a).State != "succeeded" || p.job(b).State != "succeeded" {
		t.Fatal("creates did not independently reach Ready")
	}
	sa := p.add(3, "stop", "allocation-a", ja.InstanceID)
	sb := p.add(4, "stop", "allocation-b", jb.InstanceID)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{sa, sb} {
		job := p.job(id)
		instance := c.instances[job.InstanceID]
		if job.State != "succeeded" || instance.Lifecycle != "reclaimed" || instance.Process != "stopped" || instance.Cleanup != "complete" {
			t.Fatalf("incomplete independent reclaim: %+v %+v", job, instance)
		}
	}
}

func TestOneCreateFailureDoesNotCancelIndependentCreate(t *testing.T) {
	p, c := newParallelPlatform(), newParallelCore()
	a := p.add(1, "create", "allocation-a", "")
	b := p.add(2, "create", "allocation-b", "")
	c.mode["nodejob-"+strings.ReplaceAll(a, "-", "")] = "failed"
	r := parallelRunner(t, p, c, 2)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job(a).State != "failed_with_effect" || p.job(b).State != "accepted" {
		t.Fatalf("failure crossed jobs: A=%+v B=%+v", p.job(a), p.job(b))
	}
	c.ready(p.job(b).OperationID)
	if err := r.Step(context.Background()); err != nil || p.job(b).State != "succeeded" {
		t.Fatalf("B did not finish after A failure: %v %+v", err, p.job(b))
	}
}

func TestParallelWorkerErrorIsolationAndBound(t *testing.T) {
	p, c := newParallelPlatform(), newParallelCore()
	for i := 1; i <= 20; i++ {
		p.add(i, "create", fmt.Sprintf("allocation-%d", i), "")
	}
	r := parallelRunner(t, p, c, 8)
	for i := 0; i < 3; i++ {
		if err := r.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 20; i++ {
		job := p.job(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
		if job.State != "accepted" || c.creates[job.FrozenCreate.IdempotencyKey] != 1 {
			t.Fatalf("job %d was lost or duplicated: %+v", i, job)
		}
	}
	c.operationErr[p.job("00000000-0000-0000-0000-000000000001").OperationID] = errors.New("isolated read failure")
	c.ready(p.job("00000000-0000-0000-0000-000000000002").OperationID)
	if err := r.Step(context.Background()); err == nil {
		t.Fatal("job error was hidden")
	}
	if p.job("00000000-0000-0000-0000-000000000002").State != "succeeded" {
		t.Fatal("A operation error cancelled B")
	}
}
