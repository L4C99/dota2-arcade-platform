package runner

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"

	coreclient "github.com/L4C99/dota2-arcade-dedicated-core/client"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

type fakePlatform struct {
	job      nodev1.Job
	prepared bool
	reports  []nodev1.ReportRequest
	events   *[]string
}

func (p *fakePlatform) OpenJobs(context.Context) ([]nodev1.Job, error) {
	if p.events != nil {
		*p.events = append(*p.events, "open")
	}
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
	p.job.PreparedAtUnix = time.Now().Unix()
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
	creates     int
	stops       int
	result      core.Accepted
	err         error
	op          core.Operation
	instance    core.Instance
	opErr       error
	statusErr   error
	historyDays int
	events      *[]string
	keys        []nodev1.FrozenCreate
}

func (c *fakeCore) Create(_ context.Context, frozen nodev1.FrozenCreate) (core.Accepted, error) {
	c.creates++
	c.keys = append(c.keys, frozen)
	return c.result, c.err
}
func (c *fakeCore) Stop(context.Context, string) (core.Accepted, error) {
	c.stops++
	return c.result, c.err
}
func (c *fakeCore) Operation(context.Context, string) (core.Operation, error) { return c.op, c.opErr }
func (c *fakeCore) Status(context.Context, string) (core.Instance, error) {
	return c.instance, c.statusErr
}
func (c *fakeCore) List(context.Context) (core.ListResult, error) {
	if c.events != nil {
		*c.events = append(*c.events, "list")
	}
	var out core.ListResult
	if c.historyDays == 0 {
		out.Storage.HistoryDays = 30
	} else {
		out.Storage.HistoryDays = c.historyDays
	}
	return out, nil
}

func testJob() nodev1.Job {
	return nodev1.Job{ID: "12345678-1234-1234-1234-1234567890ab", Kind: "create", State: "pending", TemplateBindingKey: "test", RequestedPort: 28000}
}

func testTemplate(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("template.json")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCreatePreparedAcceptedThenReady(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"}, op: core.Operation{OperationID: "o_1", Kind: "create", InstanceID: "i_1", Status: "succeeded"}, instance: core.Instance{InstanceID: "i_1", Port: 28000, Lifecycle: "active", Process: "running", Room: "ready"}}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": testTemplate(t)}, Network: nodev1.NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28000, MappingMode: "identity"}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !p.prepared || c.creates != 1 || p.job.State != "succeeded" || len(p.reports) != 2 || p.reports[0].State != "accepted" {
		t.Fatalf("flow: %+v %+v", p, c)
	}
	if p.reports[1].JoinInfo == nil || p.reports[1].JoinInfo.PublicPort != 28000 || p.reports[1].JoinInfoErrorCode != "" {
		t.Fatalf("Ready did not use actual port: %+v", p.reports[1])
	}
}

func TestReadyWithoutActualPortMappingReportsError(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"},
		op:       core.Operation{OperationID: "o_1", Kind: "create", InstanceID: "i_1", Status: "succeeded"},
		instance: core.Instance{InstanceID: "i_1", Port: 28001, Lifecycle: "active", Process: "running", Room: "ready"}}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": testTemplate(t)},
		Network: nodev1.NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28000, MappingMode: "identity"}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "succeeded" || p.reports[1].JoinInfo != nil ||
		p.reports[1].JoinInfoErrorCode != "PORT_MAPPING_UNAVAILABLE" || c.stops != 0 {
		t.Fatalf("Ready without mapping did not remain running: %+v", p.reports)
	}
}

func TestClaimedBeforePrepareCanStartAfterRestart(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	p.job.State = "claimed"
	c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"}, op: core.Operation{OperationID: "o_1", Kind: "create", InstanceID: "i_1", Status: "running"}}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": testTemplate(t)}, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !p.prepared || c.creates != 1 || p.job.State != "accepted" {
		t.Fatalf("pre-prepare recovery: %+v %+v", p, c)
	}
}

func TestTimeoutStaysUnknown(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	c := &fakeCore{err: errors.New("response lost")}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": testTemplate(t)}, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 1 || p.job.State != "unknown" {
		t.Fatalf("lost response: %+v %+v", p, c)
	}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 2 || c.keys[0] != c.keys[1] || p.job.State != "unknown" {
		t.Fatal("unknown create did not retry the same frozen request")
	}
}

func TestLostResponseRecoversOriginalIDsAndListsFirst(t *testing.T) {
	events := []string{}
	p := &fakePlatform{job: testJob(), events: &events}
	c := &fakeCore{err: errors.New("lost create response"), events: &events}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": testTemplate(t)}, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000}}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "unknown" {
		t.Fatalf("state = %s", p.job.State)
	}
	c.err = nil
	c.result = core.Accepted{Accepted: true, InstanceID: "i_original", OperationID: "o_original"}
	c.op = core.Operation{OperationID: "o_original", Kind: "create", InstanceID: "i_original", Status: "succeeded"}
	c.instance = core.Instance{InstanceID: "i_original", Lifecycle: "active", Process: "running", Room: "ready"}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "succeeded" || c.creates != 2 || c.keys[0] != c.keys[1] {
		t.Fatalf("recovery: %+v %+v", p, c)
	}
	if len(events) < 3 || events[0] != "list" || events[1] != "open" || events[2] != "list" {
		t.Fatalf("reconcile order: %v", events)
	}
	if events[3] != "open" {
		t.Fatalf("reconcile order: %v", events)
	}
}

func TestExpiredUnknownDoesNotReplayOrClaim(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	p.job.State = "unknown"
	p.job.PreparedAtUnix = time.Now().Add(-31 * 24 * time.Hour).Unix()
	key := "nodejob-123456781234123412341234567890ab"
	path := testTemplate(t)
	digest := nodev1.CreateFingerprint(key, path, 28000)
	p.job.FrozenCreate = &nodev1.FrozenCreate{IdempotencyKey: key, Template: path, Port: 28000, FingerprintSHA256: hex.EncodeToString(digest[:])}
	c := &fakeCore{}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 0 || p.job.State != "unknown" {
		t.Fatalf("expired unknown replayed: %+v %+v", p, c)
	}
}

func TestHistoryWindowRequiresKnownRecentPreparation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct {
		name     string
		prepared int64
		days     int
		want     bool
	}{
		{"recent", now.Add(-time.Hour).Unix(), 30, true},
		{"one-day-retention-recent", now.Add(-time.Hour).Unix(), 1, true},
		{"near-expiry", now.Add(-30*24*time.Hour + 30*time.Minute).Unix(), 30, false},
		{"unknown-time", 0, 30, false},
		{"unknown-retention", now.Unix(), 0, false},
		{"future-clock", now.Add(2 * time.Minute).Unix(), 30, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinCoreHistory(tc.prepared, tc.days, now); got != tc.want {
				t.Fatalf("withinCoreHistory = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestKnownAcceptedOperationAfterRestartDoesNotCreate(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	p.job.State, p.job.InstanceID, p.job.OperationID = "accepted", "i_1", "o_1"
	c := &fakeCore{op: core.Operation{OperationID: "o_1", Kind: "create", InstanceID: "i_1", Status: "succeeded"}, instance: core.Instance{InstanceID: "i_1", Lifecycle: "active", Process: "running", Room: "ready"}}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 0 || p.job.State != "succeeded" {
		t.Fatalf("accepted recovery: %+v %+v", p, c)
	}
}

func TestRetryRejectionCannotEraseEarlierUnknownEffect(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	p.job.State = "unknown"
	p.job.PreparedAtUnix = time.Now().Unix()
	key := "nodejob-123456781234123412341234567890ab"
	path := testTemplate(t)
	digest := nodev1.CreateFingerprint(key, path, 28000)
	p.job.FrozenCreate = &nodev1.FrozenCreate{IdempotencyKey: key, Template: path, Port: 28000, FingerprintSHA256: hex.EncodeToString(digest[:])}
	c := &fakeCore{err: &coreclient.Error{Code: "PORT_IN_USE", Stage: "validate"}}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 1 || p.job.State != "unknown" {
		t.Fatalf("retry incorrectly released unknown effect: %+v %+v", p, c)
	}
}

func TestIdempotencyConflictCannotReleaseJob(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	c := &fakeCore{err: &coreclient.Error{Code: "IDEMPOTENCY_CONFLICT", Stage: "validate"}}
	r := Runner{Platform: p, Core: c, TemplateBindings: map[string]string{"test": testTemplate(t)}, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000}}
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

func TestStopLostResponseFindsExistingStopOperation(t *testing.T) {
	p := &fakePlatform{job: nodev1.Job{ID: "12345678-1234-1234-1234-1234567890ab", Kind: "stop", State: "unknown", InstanceID: "i_1"}}
	c := &fakeCore{instance: core.Instance{InstanceID: "i_1", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete", CurrentOperationID: "o_stop"}, op: core.Operation{OperationID: "o_stop", Kind: "stop", InstanceID: "i_1", Status: "succeeded"}}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "succeeded" || p.job.OperationID != "o_stop" || c.stops != 0 {
		t.Fatalf("stop reconciliation: %+v %+v", p, c)
	}
}

func TestStopTerminalMatrix(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opStatus string
		instance core.Instance
		want     string
		code     string
	}{
		{"accepted-then-reclaimed", "succeeded", core.Instance{Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}, "succeeded", ""},
		{"failed-but-reclaimed", "failed", core.Instance{Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}, "succeeded", ""},
		{"cleanup-failed", "succeeded", core.Instance{Lifecycle: "failed", Process: "stopped", Cleanup: "failed"}, "failed_with_effect", "CLEANUP_FAILED"},
		{"identity-unknown", "succeeded", core.Instance{Lifecycle: "failed", Process: "unknown", Cleanup: "pending"}, "failed_with_effect", "IDENTITY_UNVERIFIED"},
		{"stop-failed", "failed", core.Instance{Lifecycle: "failed", Process: "running", Cleanup: "pending"}, "failed_with_effect", "STOP_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePlatform{job: nodev1.Job{ID: "12345678-1234-1234-1234-1234567890ab", Kind: "stop", State: "accepted", InstanceID: "i_1", OperationID: "o_1"}}
			tc.instance.InstanceID = "i_1"
			c := &fakeCore{op: core.Operation{OperationID: "o_1", Kind: "stop", InstanceID: "i_1", Status: tc.opStatus,
				Error: &core.CoreError{Code: "STOP_FAILED", Stage: "stop"}}, instance: tc.instance}
			r := Runner{Platform: p, Core: c}
			if err := r.Step(context.Background()); err != nil {
				t.Fatal(err)
			}
			if p.job.State != tc.want || c.stops != 0 || len(p.reports) != 1 || p.reports[0].ErrorCode != tc.code {
				t.Fatalf("stop convergence: job=%+v reports=%+v core=%+v", p.job, p.reports, c)
			}
		})
	}
}

func TestStopStatusTransportLossRemainsOpen(t *testing.T) {
	p := &fakePlatform{job: nodev1.Job{ID: "12345678-1234-1234-1234-1234567890ab", Kind: "stop", State: "unknown", InstanceID: "i_1"}}
	c := &fakeCore{statusErr: errors.New("lost status response")}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err == nil || p.job.State != "unknown" || c.stops != 0 || len(p.reports) != 0 {
		t.Fatalf("transport loss was converted to a terminal result: err=%v job=%+v reports=%+v", err, p.job, p.reports)
	}
}

func TestStopIdentityErrorQuarantinesWithoutRetry(t *testing.T) {
	p := &fakePlatform{job: nodev1.Job{ID: "12345678-1234-1234-1234-1234567890ab", Kind: "stop", State: "unknown", InstanceID: "i_1"}}
	c := &fakeCore{statusErr: &core.CoreError{Code: "IDENTITY_UNVERIFIED", Stage: "recover"}}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil || p.job.State != "failed_with_effect" || c.stops != 0 || p.reports[0].ErrorCode != "IDENTITY_UNVERIFIED" {
		t.Fatalf("untrusted identity was retried or released: err=%v job=%+v reports=%+v", err, p.job, p.reports)
	}
}

func TestStructuredStopFailureIsTerminal(t *testing.T) {
	p := &fakePlatform{job: nodev1.Job{ID: "12345678-1234-1234-1234-1234567890ab", Kind: "stop", State: "pending", InstanceID: "i_1"}}
	c := &fakeCore{instance: core.Instance{InstanceID: "i_1", Lifecycle: "active", Process: "running", Cleanup: "pending"},
		err: &core.CoreError{Code: "STOP_FAILED", Stage: "stop"}}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil || p.job.State != "failed_with_effect" || c.stops != 1 ||
		p.reports[0].ErrorCode != "STOP_FAILED" {
		t.Fatalf("structured stop failure: err=%v job=%+v reports=%+v", err, p.job, p.reports)
	}
}

func TestAcceptedCreateUntrustedIdentityDoesNotRetry(t *testing.T) {
	p := &fakePlatform{job: testJob()}
	p.job.State, p.job.InstanceID, p.job.OperationID = "accepted", "i_1", "o_1"
	c := &fakeCore{op: core.Operation{OperationID: "o_1", Kind: "create", InstanceID: "i_1", Status: "failed"},
		statusErr: &core.CoreError{Code: "IDENTITY_UNVERIFIED", Stage: "recover"}}
	r := Runner{Platform: p, Core: c}
	if err := r.Step(context.Background()); err != nil || p.job.State != "failed_with_effect" || c.creates != 0 ||
		p.reports[0].ErrorCode != "IDENTITY_UNVERIFIED" {
		t.Fatalf("untrusted create identity: err=%v job=%+v reports=%+v", err, p.job, p.reports)
	}
}
