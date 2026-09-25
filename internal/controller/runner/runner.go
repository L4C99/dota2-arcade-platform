// Package runner executes durable node jobs through the fixed d2core client.
package runner

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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
}

// Step reconciles local core state against durable work before claiming a new
// job. The caller must have established a compatible node heartbeat.
func (r *Runner) Step(ctx context.Context) error {
	list, err := r.Core.List(ctx)
	if err != nil {
		return err
	}
	jobs, err := r.Platform.OpenJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.State == "pending" {
			continue
		}
		if err := r.handle(ctx, job, false, list); err != nil {
			return err
		}
	}
	if platform, ok := r.Platform.(activeReconciler); ok {
		if err := r.reconcileActive(ctx, platform); err != nil {
			return err
		}
	}
	// Leave another claim until all existing work has converged.
	for _, job := range jobs {
		if job.State != "pending" {
			return nil
		}
	}
	job, err := r.Platform.Claim(ctx)
	if err != nil || job == nil {
		return err
	}
	return r.handle(ctx, *job, true, list)
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
			} else if instance.Lifecycle == "active" && instance.Process == "running" {
				fact.Outcome = "active"
			}
		}
		if err := platform.ReportInstanceFact(ctx, allocation.ID, fact); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) handle(ctx context.Context, job nodev1.Job, fresh bool, list core.ListResult) error {
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
		if !validCorePath(path) {
			return r.report(ctx, job, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_TEMPLATE_BINDING", ErrorStage: "validate"})
		}
		prepared, err := r.Platform.Prepare(ctx, job.ID, nodev1.PrepareCreateRequest{Template: path, Port: job.RequestedPort})
		if err != nil {
			return err
		}
		frozen = &prepared
	}
	if err := validateFrozen(job, *frozen); err != nil {
		return err
	}
	accepted, err := r.Core.Create(ctx, *frozen)
	if err != nil {
		if !retrying && core.ClearlyNoEffectCreateReject(err) {
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
	if instance.CurrentOperationID != "" {
		op, err := r.Core.Operation(ctx, instance.CurrentOperationID)
		if err != nil {
			return err
		}
		if op.InstanceID != job.InstanceID {
			return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID,
				OperationID: job.OperationID, ErrorCode: "IDENTITY_UNVERIFIED", ErrorStage: "recover"})
		}
		if op.Kind == "stop" {
			job.OperationID = op.OperationID
			if err := r.report(ctx, job, nodev1.ReportRequest{State: "accepted", InstanceID: job.InstanceID, OperationID: job.OperationID}); err != nil {
				return err
			}
			return r.observe(ctx, job)
		}
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
