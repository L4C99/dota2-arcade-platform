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
