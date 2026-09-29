package store

import (
	"context"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestV102CreateOperationStartDurablyRetainsCapacity(t *testing.T) {
	s, candidate := v102ValidationFixture(t, 1)
	ctx := context.Background()
	run, err := s.ReserveValidation(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.CreateJobID); err != nil {
		t.Fatal(err)
	}
	frozen, err := s.PrepareCreateWithManifest(ctx, candidate.NodeID, run.CreateJobID, "C:/versioned/template.json", 0,
		nodev1.TemplateManifestAlgorithmV1, candidate.ExpectedTemplateFingerprintSHA256)
	if err != nil {
		t.Fatal(err)
	}
	request := nodev1.OperationStartRequest{Kind: "create", FrozenCreate: ptrFrozenContract(frozen.Contract())}
	if _, err := s.ReportJob(ctx, candidate.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_early", OperationID: "o_early"}); err == nil {
		t.Fatal("accepted before operation-start")
	}
	wrong := request
	badFrozen := *request.FrozenCreate
	badFrozen.Port++
	wrong.FrozenCreate = &badFrozen
	if _, err := s.BeginOperation(ctx, candidate.NodeID, run.CreateJobID, wrong); err == nil {
		t.Fatal("wrong frozen execution started")
	}
	started, err := s.BeginOperation(ctx, candidate.NodeID, run.CreateJobID, request)
	if err != nil || started.State != "unknown" || started.FrozenCreate == nil || *started.FrozenCreate != frozen.Contract() {
		t.Fatalf("start: %+v %v", started, err)
	}
	var jobState, allocationState string
	var occupied, jobs int
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM node_jobs WHERE id=$1`, run.CreateJobID).Scan(&jobState); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM allocations WHERE id=$1`, run.AllocationID).Scan(&allocationState); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, candidate.NodeID).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='create'`, run.AllocationID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobState != "unknown" || allocationState != "create_unknown" || occupied != 1 || jobs != 1 {
		t.Fatalf("pre-core durable state: job=%s allocation=%s occupied=%d jobs=%d", jobState, allocationState, occupied, jobs)
	}
	if _, err := s.ReportJob(ctx, candidate.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_REJECTION"}); err == nil {
		t.Fatal("may-have-started create released as no effect")
	}
	recovered, err := s.BeginOperation(ctx, candidate.NodeID, run.CreateJobID, request)
	if err != nil || recovered.ID != run.CreateJobID || recovered.FrozenCreate == nil || *recovered.FrozenCreate != frozen.Contract() {
		t.Fatalf("crash-before-core recovery changed identity: %+v %v", recovered, err)
	}
}

func TestV102StopOperationStartUsesOriginalFormalJob(t *testing.T) {
	s, candidate, run := v102AwaitingHumanRun(t)
	ctx := context.Background()
	run, err := s.ConfirmValidationHuman(ctx, run.ID, candidate.AdminID, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.StopJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, candidate.NodeID, run.StopJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_validation", OperationID: "o_early"}); err == nil {
		t.Fatal("stop accepted before operation-start")
	}
	request := nodev1.OperationStartRequest{Kind: "stop", InstanceID: "i_validation"}
	started, err := s.BeginOperation(ctx, candidate.NodeID, run.StopJobID, request)
	if err != nil || started.State != "unknown" || started.InstanceID != request.InstanceID {
		t.Fatalf("stop start: %+v %v", started, err)
	}
	var state string
	var jobs, occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM allocations WHERE id=$1`, run.AllocationID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, run.AllocationID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, candidate.NodeID).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if state != "stopping" || jobs != 1 || occupied != 1 {
		t.Fatalf("stop crash-before-core state=%s jobs=%d occupied=%d", state, jobs, occupied)
	}
	recovered, err := s.BeginOperation(ctx, candidate.NodeID, run.StopJobID, request)
	if err != nil || recovered.ID != run.StopJobID || recovered.InstanceID != request.InstanceID {
		t.Fatalf("stop recovery changed formal Job: %+v %v", recovered, err)
	}
}

func ptrFrozenContract(value nodev1.FrozenCreate) *nodev1.FrozenCreate { return &value }
