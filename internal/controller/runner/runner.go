// Package runner executes durable node jobs through the fixed d2core client.
package runner

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/network"
)

type Platform interface {
	OpenJobs(context.Context) ([]nodev1.Job, error)
	Claim(context.Context) (*nodev1.Job, error)
	Prepare(context.Context, string, nodev1.PrepareCreateRequest) (nodev1.FrozenCreate, error)
	Report(context.Context, string, nodev1.ReportRequest) (nodev1.Job, error)
}

type activeReconciler interface {
	ActiveAllocations(context.Context) ([]nodev1.ActiveAllocation, error)
	ReportInstanceFact(context.Context, string, nodev1.InstanceFact) error
}

type Runner struct {
	Platform         Platform
	Core             core.API
	TemplateBindings map[string]string
	Network          nodev1.NetworkFacts
	TemplateProof    func(string) (string, string, error)
	ContentProof     func(string) nodev1.ContentFact
	// MaxConcurrentJobs bounds one cycle's independent core/API work. The
	// Platform's durable capacity reservation remains the create limit.
	MaxConcurrentJobs int
}

// Step inventories durable work before dispatch. The Platform claim transaction
// excludes jobs sharing an Allocation or instance with unresolved work. No
// worker survives this cycle: the next cycle (or process) reloads durable jobs.
func (r *Runner) Step(ctx context.Context) error {
	list, err := r.Core.List(ctx)
	if err != nil {
		return err
	}
	return r.StepWithList(ctx, list)
}

// StepWithList uses the exact cycle-local List result reported as inventory.
func (r *Runner) StepWithList(ctx context.Context, list core.ListResult) error {
	jobs, err := r.Platform.OpenJobs(ctx)
	if err != nil {
		return err
	}
	limit := r.MaxConcurrentJobs
	if limit < 1 {
		limit = 1
	}
	if limit > 32 {
		limit = 32
	}
	type work struct {
		job   nodev1.Job
		fresh bool
	}
	workItems := make([]work, 0, len(jobs)+limit)
	for _, job := range jobs {
		if job.State != "pending" {
			workItems = append(workItems, work{job: job})
		}
	}
	var cycleErrors []error
	// Keep the restricted stop route available while existing work is open.
	// Ordinary claims now have the same dependency guard in the Store.
	if len(workItems) > 0 {
		if platform, ok := r.Platform.(interface {
			ClaimIndependentStop(context.Context) (*nodev1.Job, error)
		}); ok {
			stop, claimErr := platform.ClaimIndependentStop(ctx)
			if claimErr != nil {
				cycleErrors = append(cycleErrors, claimErr)
			} else if stop != nil {
				if stop.Kind != "stop" || stop.InstanceID == "" {
					cycleErrors = append(cycleErrors, errors.New("invalid independent stop"))
				} else {
					// Give cleanup a slot even when all ordinary workers would
					// otherwise be occupied by slow existing creates.
					workItems = append([]work{{job: *stop, fresh: true}}, workItems...)
				}
			}
		}
	}
	claims := 0
	for claims < limit && ctx.Err() == nil {
		job, claimErr := r.Platform.Claim(ctx)
		if claimErr != nil {
			cycleErrors = append(cycleErrors, claimErr)
			break
		}
		if job == nil {
			break
		}
		workItems = append(workItems, work{job: *job, fresh: true})
		claims++
	}
	// Each job is dispatched exactly once in this cycle. A slow or failing
	// operation uses one slot but cannot cancel an independent job's work.
	workCh := make(chan work)
	var wg sync.WaitGroup
	var mu sync.Mutex
	workers := min(limit, len(workItems))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workCh {
				if err := r.handle(ctx, item.job, item.fresh, list); err != nil {
					mu.Lock()
					cycleErrors = append(cycleErrors, fmt.Errorf("node job %s: %w", item.job.ID, err))
					mu.Unlock()
				}
			}
		}()
	}
	for _, item := range workItems {
		workCh <- item
	}
	close(workCh)
	wg.Wait()
	if platform, ok := r.Platform.(activeReconciler); ok {
		if err := r.reconcileActive(ctx, platform); err != nil {
			cycleErrors = append(cycleErrors, err)
		}
	}
	return errors.Join(cycleErrors...)
}

func (r *Runner) reconcileActive(ctx context.Context, platform activeReconciler) error {
	allocations, err := platform.ActiveAllocations(ctx)
	if err != nil {
		return err
	}
	for _, allocation := range allocations {
		if allocation.HasOpenJob || allocation.InstanceID == "" {
			continue
		}
		fact := nodev1.InstanceFact{InstanceID: allocation.InstanceID, Outcome: "uncertain"}
		// Status follows the full List inventory read. A transport failure is
		// unknown; only a structured core identity error isolates the attempt.
		instance, statusErr := r.Core.Status(ctx, allocation.InstanceID)
		if statusErr != nil {
			if !coreIdentityFailure(statusErr) {
				return statusErr
			}
			fact.Outcome = "identity_unverified"
		} else if instance.InstanceID != allocation.InstanceID {
			fact.Outcome = "identity_unverified"
		} else {
			fact.Lifecycle, fact.Process, fact.Cleanup, fact.Port = instance.Lifecycle, instance.Process, instance.Cleanup, instance.Port
			if reclaimed(instance) {
				fact.Outcome = "reclaimed"
			} else if instance.Process == "unknown" {
				fact.Outcome = "identity_unverified"
			} else if instance.Lifecycle == "active" && instance.Process == "running" {
				fact.Outcome = "active"
				fact.Room = instance.Room
				if allocation.State == "running" && instance.Room == "ready" {
					fact.JoinInfo, fact.JoinInfoErrorCode = network.JoinInfo(r.Network, instance.Port)
				}
			}
		}
		if err := platform.ReportInstanceFact(ctx, allocation.ID, fact); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) handle(ctx context.Context, job nodev1.Job, fresh bool, list core.ListResult) error {
	if err := r.checkExecutionSession(ctx, job); err != nil {
		return err
	}
	switch job.Kind {
	case "create":
		return r.create(ctx, job, fresh, list.Storage.HistoryDays)
	case "stop":
		return r.stop(ctx, job)
	default:
		return fmt.Errorf("unsupported node job kind %q", job.Kind)
	}
}

func (r *Runner) create(ctx context.Context, job nodev1.Job, fresh bool, historyDays int) error {
	if job.State == "accepted" || job.State == "unknown" && job.OperationID != "" {
		return r.observe(ctx, job)
	}
	if job.State != "claimed" && job.State != "unknown" {
		return fmt.Errorf("invalid create job state %q", job.State)
	}
	retrying := job.FrozenCreate != nil && !fresh
	if job.State == "unknown" && job.FrozenCreate == nil {
		return errors.New("unknown create lacks frozen request")
	}
	if retrying && !withinCoreHistory(job.PreparedAtUnix, historyDays, time.Now()) {
		if job.State == "claimed" {
			return r.report(ctx, job, nodev1.ReportRequest{State: "unknown"})
		}
		return nil
	}
	frozen := job.FrozenCreate
	if frozen == nil {
		if job.RequestedPort != 0 && (job.RequestedPort < r.Network.LocalPortMin || job.RequestedPort > r.Network.LocalPortMax) {
			return r.report(ctx, job, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_PORT_RANGE", ErrorStage: "validate"})
		}
		path := r.TemplateBindings[job.TemplateBindingKey]
		manifestSHA := ""
		if job.RequiredCapability == nodev1.RequiredContentValidationV102 {
			var proofErr error
			path, manifestSHA, proofErr = r.verifyMachineProof(job)
			if proofErr != nil {
				return r.report(ctx, job, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_IDENTITY_MISMATCH", ErrorStage: "validate"})
			}
		}
		if !validCorePath(path) {
			return r.report(ctx, job, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_TEMPLATE_BINDING", ErrorStage: "validate"})
		}
		request := nodev1.PrepareCreateRequest{Template: path, Port: job.RequestedPort}
		if job.RequiredCapability == nodev1.RequiredContentValidationV102 {
			request.TemplateManifestAlgorithm, request.ObservedTemplateFingerprintSHA256 = nodev1.TemplateManifestAlgorithmV1, manifestSHA
		}
		prepared, err := r.Platform.Prepare(ctx, job.ID, request)
		if err != nil {
			return err
		}
		frozen = &prepared
	}
	if err := validateFrozen(job, *frozen); err != nil {
		return err
	}
	if job.RequiredCapability == nodev1.RequiredContentValidationV102 {
		path, digest, err := r.verifyMachineProof(job)
		if err != nil || path != frozen.Template || digest != frozen.TemplateFingerprintSHA256 {
			if !retrying {
				return r.report(ctx, job, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_IDENTITY_DRIFT", ErrorStage: "validate"})
			}
			if job.State == "claimed" {
				return r.report(ctx, job, nodev1.ReportRequest{State: "unknown", ErrorCode: "LOCAL_IDENTITY_DRIFT", ErrorStage: "reconcile"})
			}
			return errors.New("local machine identity drift after possible core call")
		}
		if err := r.checkExecutionSession(ctx, job); err != nil {
			return err
		}
		if err := r.beginExecution(ctx, job, frozen); err != nil {
			return err
		}
	}
	accepted, err := r.Core.Create(ctx, *frozen)
	if err != nil {
		if job.RequiredCapability != nodev1.RequiredContentValidationV102 && !retrying && core.ClearlyNoEffectCreateReject(err) {
			var e *core.CoreError
			_ = errors.As(err, &e)
			return r.report(ctx, job, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: e.Code, ErrorStage: e.Stage})
		}
		var report = nodev1.ReportRequest{State: "unknown"}
		var e *core.CoreError
		if errors.As(err, &e) {
			report.InstanceID, report.OperationID, report.ErrorCode, report.ErrorStage = e.InstanceID, e.OperationID, e.Code, e.Stage
		}
		return r.report(ctx, job, report)
	}
	job.InstanceID, job.OperationID = accepted.InstanceID, accepted.OperationID
	if err := r.report(ctx, job, nodev1.ReportRequest{State: "accepted", InstanceID: job.InstanceID, OperationID: job.OperationID}); err != nil {
		return err
	}
	return r.observe(ctx, job)
}

// An old key is not proof after d2core may have expired reclaimed history.
// One hour is reserved for clock skew and online cleanup timing.
func withinCoreHistory(preparedAt int64, historyDays int, now time.Time) bool {
	if preparedAt <= 0 || historyDays < 1 {
		return false
	}
	prepared := time.Unix(preparedAt, 0)
	return !prepared.After(now.Add(time.Minute)) && now.Sub(prepared) < time.Duration(historyDays)*24*time.Hour-time.Hour
}

func (r *Runner) stop(ctx context.Context, job nodev1.Job) error {
	if err := r.checkExecutionSession(ctx, job); err != nil {
		return err
	}
	if job.State == "accepted" || job.State == "unknown" && job.OperationID != "" {
		return r.observe(ctx, job)
	}
	if job.State != "claimed" && job.State != "unknown" {
		return fmt.Errorf("invalid stop job state %q", job.State)
	}
	instance, err := r.Core.Status(ctx, job.InstanceID)
	if err != nil {
		if terminalCoreFailure(err) || coreIdentityFailure(err) {
			code := coreFailureCode(err)
			if code == "NOT_FOUND" {
				code = "IDENTITY_UNVERIFIED"
			}
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: code, ErrorStage: coreFailureStage(err)})
		}
		return err
	}
	if instance.InstanceID != job.InstanceID {
		return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
			OperationID: job.OperationID, ErrorCode: "IDENTITY_UNVERIFIED", ErrorStage: "recover"})
	}
	oldTerminalOperation := ""
	if instance.CurrentOperationID != "" {
		op, err := r.Core.Operation(ctx, instance.CurrentOperationID)
		if err != nil {
			return err
		}
		if op.InstanceID != job.InstanceID {
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: "IDENTITY_UNVERIFIED", ErrorStage: "recover"})
		}
		if op.Kind == "stop" && (op.Status == "running" || reclaimed(instance)) {
			job.OperationID = op.OperationID
			if err := r.report(ctx, job, nodev1.ReportRequest{State: "accepted", InstanceID: job.InstanceID, OperationID: job.OperationID}); err != nil {
				return err
			}
			return r.observe(ctx, job)
		}
		if op.Kind == "stop" && (op.Status == "failed" || op.Status == "cancelled" || op.Status == "succeeded") {
			oldTerminalOperation = op.OperationID
		}
	}
	if err := r.checkExecutionSession(ctx, job); err != nil {
		return err
	}
	if err := r.beginExecution(ctx, job, nil); err != nil {
		return err
	}
	accepted, err := r.Core.Stop(ctx, job.InstanceID)
	if err != nil {
		if terminalCoreFailure(err) || coreIdentityFailure(err) {
			code := coreFailureCode(err)
			if code == "NOT_FOUND" {
				code = "IDENTITY_UNVERIFIED"
			}
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: code, ErrorStage: coreFailureStage(err)})
		}
		report := nodev1.ReportRequest{State: "unknown", InstanceID: job.InstanceID}
		var e *core.CoreError
		if errors.As(err, &e) && (e.InstanceID == "" || e.InstanceID == job.InstanceID) {
			report.OperationID, report.ErrorCode, report.ErrorStage = e.OperationID, e.Code, e.Stage
		}
		return r.report(ctx, job, report)
	}
	if accepted.InstanceID != job.InstanceID {
		return fmt.Errorf("core stop returned different instance ID")
	}
	// v0.1.1 can still return the finished worker's operation until teardown
	// removes the worker. Do not freeze that old failure into this new job.
	// Retain this job without an operation ID and retry on the next cycle.
	if oldTerminalOperation != "" && accepted.OperationID == oldTerminalOperation {
		return r.report(ctx, job, nodev1.ReportRequest{State: "unknown", InstanceID: job.InstanceID})
	}
	job.OperationID = accepted.OperationID
	if err := r.report(ctx, job, nodev1.ReportRequest{State: "accepted", InstanceID: job.InstanceID, OperationID: job.OperationID}); err != nil {
		return err
	}
	return r.observe(ctx, job)
}

func (r *Runner) observe(ctx context.Context, job nodev1.Job) error {
	if job.OperationID == "" || job.InstanceID == "" {
		return nil
	}
	if job.Kind == "create" && job.RequiredCapability == nodev1.RequiredContentValidationV102 {
		if _, _, err := r.verifyMachineProof(job); err != nil {
			return r.report(ctx, job, nodev1.ReportRequest{State: "unknown", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: "LOCAL_IDENTITY_DRIFT", ErrorStage: "reconcile"})
		}
	}
	op, err := r.Core.Operation(ctx, job.OperationID)
	if err != nil {
		if job.Kind == "stop" && (terminalCoreFailure(err) || coreIdentityFailure(err)) {
			code := coreFailureCode(err)
			if code == "NOT_FOUND" {
				code = "IDENTITY_UNVERIFIED"
			}
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: code, ErrorStage: coreFailureStage(err)})
		}
		if job.Kind == "create" && coreIdentityFailure(err) {
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: "IDENTITY_UNVERIFIED", ErrorStage: "recover"})
		}
		return err
	}
	if op.InstanceID != job.InstanceID || op.Kind != job.Kind {
		return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
			OperationID: job.OperationID, ErrorCode: "IDENTITY_UNVERIFIED", ErrorStage: "recover"})
	}
	if op.Status == "running" {
		return nil
	}
	instance, err := r.Core.Status(ctx, job.InstanceID)
	if err != nil {
		if job.Kind == "stop" && (terminalCoreFailure(err) || coreIdentityFailure(err)) {
			code := coreFailureCode(err)
			if code == "NOT_FOUND" {
				code = "IDENTITY_UNVERIFIED"
			}
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: code, ErrorStage: coreFailureStage(err)})
		}
		if job.Kind == "create" && coreIdentityFailure(err) {
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: "IDENTITY_UNVERIFIED", ErrorStage: "recover"})
		}
		return err
	}
	if instance.InstanceID != job.InstanceID {
		return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
			OperationID: job.OperationID, ErrorCode: "IDENTITY_UNVERIFIED", ErrorStage: "recover"})
	}
	// Resource reclamation is a status fact, independent of whether the
	// operation itself reported success. A failed operation can still have
	// completed reclaim; conversely a succeeded operation cannot release a
	// reservation while cleanup is incomplete.
	if job.Kind == "stop" && reclaimed(instance) {
		return r.report(ctx, job, nodev1.ReportRequest{State: "succeeded", InstanceID: job.InstanceID, OperationID: job.OperationID})
	}
	if job.Kind == "stop" && (instance.Cleanup == "failed" || instance.Process == "unknown") {
		code := "IDENTITY_UNVERIFIED"
		if instance.Cleanup == "failed" {
			code = "CLEANUP_FAILED"
		}
		return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
			OperationID: job.OperationID, ErrorCode: code, ErrorStage: "cleanup"})
	}
	if op.Status == "succeeded" {
		if job.Kind == "create" && (instance.Process == "unknown" || instance.Lifecycle == "failed" || instance.Process == "stopped") {
			code := "INSTANCE_FAILED"
			if instance.Process == "unknown" {
				code = "IDENTITY_UNVERIFIED"
			}
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: code, ErrorStage: "recover"})
		}
		if job.Kind == "create" && instance.Lifecycle == "active" && instance.Process == "running" && instance.Room == "ready" {
			join, joinError := network.JoinInfo(r.Network, instance.Port)
			return r.report(ctx, job, nodev1.ReportRequest{State: "succeeded", InstanceID: job.InstanceID,
				OperationID: job.OperationID, JoinInfo: join, JoinInfoErrorCode: joinError})
		}
		if job.Kind == "stop" && reclaimed(instance) {
			return r.report(ctx, job, nodev1.ReportRequest{State: "succeeded", InstanceID: job.InstanceID, OperationID: job.OperationID})
		}
		return nil
	}
	if op.Status == "failed" || op.Status == "cancelled" {
		code, stage := "", ""
		if op.Error != nil {
			code, stage = op.Error.Code, op.Error.Stage
		}
		return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
			OperationID: job.OperationID, ErrorCode: code, ErrorStage: stage})
	}
	return fmt.Errorf("unknown core operation status %q", op.Status)
}

func terminalCoreFailure(err error) bool {
	var e *core.CoreError
	return errors.As(err, &e) && (e.Code == "IDENTITY_UNVERIFIED" || e.Code == "CLEANUP_FAILED" || e.Code == "STOP_FAILED")
}

func coreIdentityFailure(err error) bool {
	var e *core.CoreError
	return errors.As(err, &e) && (e.Code == "IDENTITY_UNVERIFIED" || e.Code == "NOT_FOUND")
}

func coreFailureCode(err error) string {
	var e *core.CoreError
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func coreFailureStage(err error) string {
	var e *core.CoreError
	if errors.As(err, &e) {
		return e.Stage
	}
	return ""
}

func reclaimed(i core.Instance) bool {
	return i.Lifecycle == "reclaimed" && i.Process == "stopped" && i.Cleanup == "complete"
}

func (r *Runner) report(ctx context.Context, job nodev1.Job, report nodev1.ReportRequest) error {
	_, err := r.Platform.Report(ctx, job.ID, report)
	return err
}

func validateFrozen(job nodev1.Job, f nodev1.FrozenCreate) error {
	if f.IdempotencyKey != "nodejob-"+strings.ReplaceAll(job.ID, "-", "") || !validCorePath(f.Template) || f.Port < 0 || f.Port > 65535 {
		return errors.New("invalid frozen core create request")
	}
	digest := nodev1.CreateFingerprint(f.IdempotencyKey, f.Template, f.Port)
	if f.FingerprintSHA256 != hex.EncodeToString(digest[:]) {
		return errors.New("frozen core create fingerprint mismatch")
	}
	if job.RequiredCapability == nodev1.RequiredContentValidationV102 && (f.RequiredCapability != job.RequiredCapability ||
		f.TemplateManifestAlgorithm != nodev1.TemplateManifestAlgorithmV1 || f.TemplateFingerprintSHA256 != job.ExpectedTemplateFingerprintSHA256) {
		return errors.New("frozen machine identity mismatch")
	}
	return nil
}

func (r *Runner) verifyMachineProof(job nodev1.Job) (string, string, error) {
	if r.TemplateProof == nil || r.ContentProof == nil || job.ExpectedTemplateFingerprintSHA256 == "" ||
		job.TemplateBindingGeneration < 1 || job.ExpectedWorkshopID == "" || job.ExpectedContentVersionID == "" || job.ExpectedVPKSHA256 == "" {
		return "", "", errors.New("incomplete machine proof")
	}
	path, digest, err := r.TemplateProof(job.TemplateBindingKey)
	if err != nil || digest != job.ExpectedTemplateFingerprintSHA256 {
		return "", "", errors.New("template identity mismatch")
	}
	fact := r.ContentProof(job.ExpectedWorkshopID)
	if fact.State != "confirmed" || fact.WorkshopID != job.ExpectedWorkshopID || fact.ContentVersionID != job.ExpectedContentVersionID || fact.VPKSHA256 != job.ExpectedVPKSHA256 {
		return "", "", errors.New("content identity mismatch")
	}
	return path, digest, nil
}

func (r *Runner) checkExecutionSession(ctx context.Context, job nodev1.Job) error {
	if job.RequiredCapability == "" || job.RequiredCapability == "legacy_v1" {
		return nil
	}
	if job.RequiredCapability != nodev1.RequiredContentValidationV102 {
		return errors.New("unsupported execution capability")
	}
	checker, ok := r.Platform.(interface{ CheckSession(context.Context) error })
	if !ok {
		return errors.New("capability session checker unavailable")
	}
	return checker.CheckSession(ctx)
}

func (r *Runner) beginExecution(ctx context.Context, job nodev1.Job, frozen *nodev1.FrozenCreate) error {
	if job.RequiredCapability != nodev1.RequiredContentValidationV102 {
		return nil
	}
	platform, ok := r.Platform.(interface {
		BeginOperation(context.Context, string, nodev1.OperationStartRequest) (nodev1.Job, error)
	})
	if !ok {
		return errors.New("durable operation-start unavailable")
	}
	request := nodev1.OperationStartRequest{Kind: job.Kind, FrozenCreate: frozen}
	if job.Kind == "stop" {
		request.InstanceID = job.InstanceID
	}
	started, err := platform.BeginOperation(ctx, job.ID, request)
	if err != nil {
		return err
	}
	if started.ID != job.ID || started.Kind != job.Kind || started.State != "unknown" ||
		(job.Kind == "create" && (started.FrozenCreate == nil || *started.FrozenCreate != *frozen)) ||
		(job.Kind == "stop" && started.InstanceID != job.InstanceID) {
		return errors.New("invalid durable operation-start response")
	}
	return nil
}

func validCorePath(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	for _, r := range path {
		if r < 32 || r > 127 {
			return false
		}
	}
	return true
}
