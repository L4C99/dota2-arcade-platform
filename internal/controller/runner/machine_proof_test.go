package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

func machineJob() nodev1.Job {
	j := testJob()
	j.RequiredCapability = nodev1.RequiredContentValidationV102
	j.ExpectedTemplateFingerprintSHA256 = strings.Repeat("f", 64)
	j.TemplateBindingGeneration = 1
	j.ExpectedWorkshopID = "123"
	j.ExpectedContentVersionID = "candidate-v1"
	j.ExpectedVPKSHA256 = strings.Repeat("a", 64)
	return j
}

func TestMachineProofUnknownRecoveryKeepsFrozenKey(t *testing.T) {
	p := &fakePlatform{job: machineJob()}
	c := &fakeCore{err: errors.New("response lost")}
	digest := strings.Repeat("f", 64)
	content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
	r := machineRunner(t, p, c, &digest, &content)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "unknown" || c.creates != 1 || p.job.FrozenCreate == nil {
		t.Fatal("first create not frozen unknown")
	}
	c.err = nil
	c.result = core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"}
	c.op = core.Operation{OperationID: "o_1", Kind: "create", InstanceID: "i_1", Status: "running"}
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 2 || len(c.keys) != 2 || c.keys[0] != c.keys[1] || p.job.State != "accepted" {
		t.Fatalf("recovery changed key or execution: %+v %+v", p, c)
	}
}

func TestCapabilityLossBlocksLocalCoreCreateAndStop(t *testing.T) {
	for _, kind := range []string{"create", "stop"} {
		t.Run(kind, func(t *testing.T) {
			job := machineJob()
			job.Kind = kind
			if kind == "stop" {
				job.InstanceID = "i_1"
			}
			p := &fakePlatform{job: job, sessionErr: errors.New("session expired")}
			c := &fakeCore{}
			digest := strings.Repeat("f", 64)
			content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
			r := machineRunner(t, p, c, &digest, &content)
			if err := r.Step(context.Background()); err == nil {
				t.Fatal("expired session allowed execution")
			}
			if c.creates != 0 || c.stops != 0 || p.prepared || len(p.reports) != 0 || p.job.State != "claimed" {
				t.Fatalf("session loss changed durable work: %+v %+v", p, c)
			}
		})
	}
}

func TestOperationStartResponseLostBeforeCoreRecoversSameCreate(t *testing.T) {
	p := &fakePlatform{job: machineJob(), afterCommitErr: errors.New("operation-start response lost")}
	c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"},
		op: core.Operation{OperationID: "o_1", Kind: "create", InstanceID: "i_1", Status: "running"}}
	digest := strings.Repeat("f", 64)
	content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
	r := machineRunner(t, p, c, &digest, &content)
	if err := r.Step(context.Background()); err == nil || p.job.State != "unknown" || c.creates != 0 || p.job.FrozenCreate == nil {
		t.Fatalf("uncertain start response called core or lost durable Job: %+v %+v %v", p, c, err)
	}
	key := p.job.FrozenCreate.IdempotencyKey
	p.afterCommitErr = nil
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 1 || c.keys[0].IdempotencyKey != key || p.job.State != "accepted" || len(p.starts) != 2 {
		t.Fatalf("recovery changed Job/key: %+v %+v", p, c)
	}
}

func TestStopOperationStartResponseLostBeforeCoreRecoversSameJob(t *testing.T) {
	job := machineJob()
	job.Kind, job.InstanceID = "stop", "i_1"
	p := &fakePlatform{job: job, afterCommitErr: errors.New("operation-start response lost")}
	c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_stop"},
		op:       core.Operation{OperationID: "o_stop", Kind: "stop", InstanceID: "i_1", Status: "running"},
		instance: core.Instance{InstanceID: "i_1", Lifecycle: "active", Process: "running", Cleanup: "pending"}}
	digest := strings.Repeat("f", 64)
	content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
	r := machineRunner(t, p, c, &digest, &content)
	if err := r.Step(context.Background()); err == nil || p.job.State != "unknown" || c.stops != 0 {
		t.Fatalf("uncertain stop start response called core: %+v %+v %v", p, c, err)
	}
	p.afterCommitErr = nil
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.stops != 1 || len(p.starts) != 2 || p.starts[0] != p.starts[1] || p.job.InstanceID != "i_1" {
		t.Fatalf("stop recovery changed formal Job/instance: %+v %+v", p, c)
	}
}

func TestV102CoreNoEffectRejectionAfterMarkerRemainsUnknown(t *testing.T) {
	p := &fakePlatform{job: machineJob()}
	c := &fakeCore{err: &core.CoreError{Code: "INVALID_ARGUMENT", Stage: "validate"}}
	digest := strings.Repeat("f", 64)
	content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
	r := machineRunner(t, p, c, &digest, &content)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 1 || len(p.starts) != 1 || p.job.State != "unknown" {
		t.Fatalf("core rejection released post-marker create: %+v %+v", p, c)
	}
	for _, report := range p.reports {
		if report.State == "rejected_no_effect" {
			t.Fatalf("no-effect report after marker: %+v", report)
		}
	}
}

func TestOperationStartDeniedAfterReplacementBeforeCore(t *testing.T) {
	p := &fakePlatform{job: machineJob()}
	p.afterPrepare = func() { p.startErr = errors.New("session replaced before operation-start") }
	c := &fakeCore{}
	digest := strings.Repeat("f", 64)
	content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
	r := machineRunner(t, p, c, &digest, &content)
	if err := r.Step(context.Background()); err == nil || c.creates != 0 || p.job.State != "claimed" {
		t.Fatalf("replacement before start called core: %+v %+v %v", p, c, err)
	}
}

func TestOperationStartThenSessionTransitionAllowsOnlyAccountedEffect(t *testing.T) {
	for _, kind := range []string{"create", "stop"} {
		for _, transition := range []string{"replacement", "expiry"} {
			t.Run(kind+"/"+transition, func(t *testing.T) {
				job := machineJob()
				job.Kind = kind
				if kind == "stop" {
					job.InstanceID = "i_1"
				}
				p := &fakePlatform{job: job}
				c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"},
					op:       core.Operation{OperationID: "o_1", Kind: kind, InstanceID: "i_1", Status: "running"},
					instance: core.Instance{InstanceID: "i_1", Lifecycle: "active", Process: "running", Cleanup: "pending"}}
				digest := strings.Repeat("f", 64)
				content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
				p.afterStart = func() { p.reportErr = errors.New("session " + transition) }
				c.afterCreate = func() {
					if p.job.State != "unknown" {
						t.Fatal("create reached core before durable may-have-started")
					}
				}
				c.beforeStop = func() {
					if p.job.State != "unknown" {
						t.Fatal("stop reached core before durable may-have-started")
					}
				}
				r := machineRunner(t, p, c, &digest, &content)
				if err := r.Step(context.Background()); err == nil || p.job.State != "unknown" || len(p.starts) != 1 ||
					(kind == "create" && c.creates != 1) || (kind == "stop" && c.stops != 1) {
					t.Fatalf("post-start transition lost accounted effect: %+v %+v %v", p, c, err)
				}
				frozen := p.job.FrozenCreate
				p.reportErr, p.afterStart = nil, nil
				if err := r.Step(context.Background()); err != nil {
					t.Fatal(err)
				}
				if kind == "create" && (c.creates != 2 || c.keys[0] != c.keys[1] || p.job.FrozenCreate != frozen) {
					t.Fatalf("create retry changed frozen identity: %+v %+v", p, c)
				}
				if kind == "stop" && (c.stops != 2 || p.job.InstanceID != "i_1") {
					t.Fatalf("stop retry changed formal target: %+v %+v", p, c)
				}
			})
		}
	}
}

func machineRunner(t *testing.T, p *fakePlatform, c *fakeCore, digest *string, content *nodev1.ContentFact) Runner {
	t.Helper()
	path := testTemplate(t)
	return Runner{Platform: p, Core: c, Network: nodev1.NetworkFacts{LocalPortMin: 28000, LocalPortMax: 28000},
		TemplateProof: func(string) (string, string, error) { return path, *digest, nil },
		ContentProof:  func(string) nodev1.ContentFact { return *content }}
}

func TestMachineProofPreCoreMismatchIsNoEffect(t *testing.T) {
	for _, scenario := range []string{"template", "version", "sha", "unknown"} {
		t.Run(scenario, func(t *testing.T) {
			p := &fakePlatform{job: machineJob()}
			c := &fakeCore{}
			digest := strings.Repeat("f", 64)
			content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
			switch scenario {
			case "template":
				digest = strings.Repeat("b", 64)
			case "version":
				content.ContentVersionID = "other"
			case "sha":
				content.VPKSHA256 = strings.Repeat("b", 64)
			case "unknown":
				content.State = "unknown"
			}
			r := machineRunner(t, p, c, &digest, &content)
			if err := r.Step(context.Background()); err != nil {
				t.Fatal(err)
			}
			if p.job.State != "rejected_no_effect" || c.creates != 0 || p.prepared {
				t.Fatalf("unsafe pre-core result: %+v %+v", p, c)
			}
		})
	}
}

func TestMachineProofDriftAfterPrepareBeforeCore(t *testing.T) {
	p := &fakePlatform{job: machineJob()}
	c := &fakeCore{}
	digest := strings.Repeat("f", 64)
	content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
	p.afterPrepare = func() { digest = strings.Repeat("b", 64) }
	r := machineRunner(t, p, c, &digest, &content)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !p.prepared || p.job.State != "rejected_no_effect" || c.creates != 0 {
		t.Fatalf("pre-core drift: %+v %+v", p, c)
	}
}

func TestMachineProofDriftAfterPossibleCoreCallRetainsUnknown(t *testing.T) {
	p := &fakePlatform{job: machineJob()}
	c := &fakeCore{err: errors.New("response lost")}
	digest := strings.Repeat("f", 64)
	content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
	r := machineRunner(t, p, c, &digest, &content)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.job.State != "unknown" || c.creates != 1 || p.job.FrozenCreate == nil {
		t.Fatalf("first call not unknown: %+v %+v", p, c)
	}
	key := p.job.FrozenCreate.IdempotencyKey
	digest = strings.Repeat("b", 64)
	_ = r.Step(context.Background())
	if p.job.State != "unknown" || c.creates != 1 || p.job.FrozenCreate.IdempotencyKey != key {
		t.Fatalf("post-call drift released or duplicated: %+v %+v", p, c)
	}
	for _, report := range p.reports {
		if report.State == "rejected_no_effect" {
			t.Fatal("post-call no-effect report")
		}
	}
}

func TestMachineProofDriftAfterAcceptedBeforeObservation(t *testing.T) {
	p := &fakePlatform{job: machineJob()}
	digest := strings.Repeat("f", 64)
	content := nodev1.ContentFact{WorkshopID: "123", ContentVersionID: "candidate-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}
	c := &fakeCore{result: core.Accepted{Accepted: true, InstanceID: "i_1", OperationID: "o_1"}, afterCreate: func() { content.VPKSHA256 = strings.Repeat("b", 64) }}
	r := machineRunner(t, p, c, &digest, &content)
	if err := r.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.creates != 1 || p.job.State != "unknown" {
		t.Fatalf("post-accept drift advanced Job: %+v %+v", p, c)
	}
	for _, report := range p.reports {
		if report.State == "succeeded" || report.State == "rejected_no_effect" {
			t.Fatalf("unsafe drift report: %+v", report)
		}
	}
}
