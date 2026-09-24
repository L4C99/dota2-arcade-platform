// Package runner executes durable node jobs through the fixed d2core client.
package runner

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

type Platform interface {
	OpenJobs(context.Context) ([]nodev1.Job, error)
	Claim(context.Context) (*nodev1.Job, error)
	Prepare(context.Context, string, nodev1.PrepareCreateRequest) (nodev1.FrozenCreate, error)
	Report(context.Context, string, nodev1.ReportRequest) (nodev1.Job, error)
}

type Runner struct {
	Platform         Platform
	Core             core.API
	TemplateBindings map[string]string
	Network          nodev1.NetworkFacts
}

// Step reads existing durable work before claiming another job. The caller
// must have established local protocol v1 and a compatible node heartbeat.
func (r *Runner) Step(ctx context.Context) error {
	jobs, err := r.Platform.OpenJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.State == "pending" {
			continue
		}
		if err := r.handle(ctx, job, false); err != nil {
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
	return r.handle(ctx, *job, true)
}

func (r *Runner) handle(ctx context.Context, job nodev1.Job, fresh bool) error {
	switch job.Kind {
	case "create":
		return r.create(ctx, job, fresh)
	case "stop":
		return r.stop(ctx, job, fresh)
	default:
		return fmt.Errorf("unsupported node job kind %q", job.Kind)
	}
}

func (r *Runner) create(ctx context.Context, job nodev1.Job, fresh bool) error {
	if job.State == "accepted" || job.State == "unknown" && job.OperationID != "" {
		return r.observe(ctx, job)
	}
	if job.State == "unknown" {
		return nil
	}
	if !fresh || job.State != "claimed" {
		// A persisted claimed job may have called core before a crash. P0E
		// reconciles the fixed key; a blind replay is not safe here.
		return r.report(ctx, job, nodev1.ReportRequest{State: "unknown", InstanceID: job.InstanceID, OperationID: job.OperationID})
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
		if core.ClearlyNoEffectCreateReject(err) {
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

func (r *Runner) stop(ctx context.Context, job nodev1.Job, fresh bool) error {
	if job.State == "accepted" || job.State == "unknown" && job.OperationID != "" {
		return r.observe(ctx, job)
	}
	if job.State == "unknown" {
		return nil
	}
	if !fresh || job.State != "claimed" {
		return r.report(ctx, job, nodev1.ReportRequest{State: "unknown", InstanceID: job.InstanceID, OperationID: job.OperationID})
	}
	accepted, err := r.Core.Stop(ctx, job.InstanceID)
	if err != nil {
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
		return err
	}
	if op.InstanceID != job.InstanceID || op.Kind != job.Kind {
		return fmt.Errorf("core operation identity mismatch for job %s", job.ID)
	}
	if op.Status == "running" {
		return nil
	}
	instance, err := r.Core.Status(ctx, job.InstanceID)
	if err != nil {
		return err
	}
	if instance.InstanceID != job.InstanceID {
		return fmt.Errorf("core status identity mismatch for job %s", job.ID)
	}
	if op.Status == "succeeded" {
		if job.Kind == "create" && instance.Lifecycle == "active" && instance.Process == "running" && instance.Room == "ready" {
			return r.report(ctx, job, nodev1.ReportRequest{State: "succeeded", InstanceID: job.InstanceID, OperationID: job.OperationID})
		}
		if job.Kind == "stop" && reclaimed(instance) {
			return r.report(ctx, job, nodev1.ReportRequest{State: "succeeded", InstanceID: job.InstanceID, OperationID: job.OperationID})
		}
		return nil
	}
	if op.Status == "failed" || op.Status == "cancelled" {
		return r.report(ctx, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: job.InstanceID, OperationID: job.OperationID})
	}
	return fmt.Errorf("unknown core operation status %q", op.Status)
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
