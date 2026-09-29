package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/buildinfo"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/config"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/network"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/ownership"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/platformclient"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/runner"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") {
		fmt.Println("usage: node-controller version | check|heartbeat|run --config ABS_PATH")
		return nil
	}
	if len(args) == 1 && args[0] == "version" {
		return buildinfo.Write(os.Stdout, "node-controller")
	}
	if len(args) != 3 || args[1] != "--config" || (args[0] != "heartbeat" && args[0] != "run" && args[0] != "check") {
		return errors.New("usage: node-controller check|heartbeat|run --config ABS_PATH")
	}
	conf, secret, err := config.Load(args[2])
	if err != nil {
		return err
	}
	if args[0] == "check" {
		fmt.Println("controller config and secret format valid; no service contacted")
		return nil
	}
	owner, err := ownership.Acquire(conf.D2CoreDataDir, conf.NodeID)
	if err != nil {
		return fmt.Errorf("Controller ownership unavailable: %w", err)
	}
	defer owner.Close()
	client := platformclient.New(conf.PlatformURL, conf.NodeID, secret)
	coreClient, err := core.New(conf.D2CoreDataDir)
	if err != nil {
		return err
	}
	// One slot per configured instance, with a second slot on single-capacity
	// nodes so an independent stop can progress beside an unresolved create.
	worker := &runner.Runner{Platform: client, Core: coreClient, TemplateBindings: conf.TemplateBindings,
		Network: conf.Network, TemplateProof: conf.TemplateManifestSHA256V1, ContentProof: conf.ContentFact,
		MaxConcurrentJobs: min(max(conf.HardMaxInstances, 2), 32)}
	factReader := config.NewFactReader(conf)
	send := func(ctx context.Context) (nodev1.HeartbeatResult, core.ListResult, bool, error) {
		// Session establishment precedes inventory and heartbeat; a failure
		// leaves all v1.0.2 execution routes closed while legacy work survives.
		sessionErr := client.EnsureSession(ctx)
		protocol := 0
		list, listErr := coreClient.List(ctx)
		var scan [16]byte
		if _, err := rand.Read(scan[:]); err != nil {
			return nodev1.HeartbeatResult{}, core.ListResult{}, false, err
		}
		scanID := hex.EncodeToString(scan[:])
		inventory := inventoryForList(scanID, list, listErr)
		if listErr == nil {
			protocol = 1
		}
		facts := factReader.Facts(buildinfo.Version, protocol)
		facts.InventoryScanID, facts.InventoryState = scanID, "unknown"
		inventoryErr := sessionErr
		if inventoryErr == nil {
			inventoryErr = client.ReportInventory(ctx, inventory)
		}
		if inventoryErr == nil && inventory.Complete {
			facts.InventoryState = "confirmed"
		} else {
			client.DropSession()
		}
		if conf.Network.A2SEnabled && listErr == nil {
			facts.A2SDiagnostics = network.ProbeReadyInstances(ctx, list.Instances)
			facts.A2SQueryOK = len(facts.A2SDiagnostics) > 0
			for _, diagnostic := range facts.A2SDiagnostics {
				if diagnostic.Status != "ok" {
					facts.A2SQueryOK = false
				}
			}
		}
		result, err := client.Heartbeat(ctx, facts)
		return result, list, listErr == nil && inventory.Complete, err
	}
	if args[0] == "heartbeat" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		result, _, _, err := send(ctx)
		if err != nil {
			return err
		}
		fmt.Println("heartbeat accepted; compatibility=" + result.CompatibilityStatus)
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastStatus := ""
	for {
		cycle, cancel := context.WithTimeout(ctx, 20*time.Second)
		result, list, listOK, err := send(cycle)
		status := result.CompatibilityStatus
		if err != nil {
			log.Printf("heartbeat failed: %v", err)
		} else if status != lastStatus {
			log.Printf("node compatibility: %s", status)
			lastStatus = status
		}
		if err == nil && status == "compatible" && listOK {
			jobCycle, cancelJobs := context.WithTimeout(ctx, 20*time.Second)
			if err := worker.StepWithList(jobCycle, list); err != nil {
				log.Printf("node job cycle failed: %v", err)
			} else if result.ReconcileRequestedGeneration > result.ReconcileCompletedGeneration {
				if err := client.CompleteReconcile(jobCycle, result.ReconcileRequestedGeneration); err != nil {
					log.Printf("node reconcile acknowledgement failed: %v", err)
				}
			}
			cancelJobs()
		}
		cancel()
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func inventoryForList(scanID string, list core.ListResult, listErr error) nodev1.InventoryReport {
	report := nodev1.InventoryReport{ScanID: scanID, Complete: listErr == nil, Instances: []nodev1.InventoryInstance{}}
	if listErr != nil {
		report.ErrorCode = "CORE_LIST_FAILED"
		return report
	}
	if list.Instances == nil {
		report.Complete, report.ErrorCode = false, "CORE_LIST_INVALID"
		return report
	}
	for _, instance := range list.Instances {
		if instance.Lifecycle == "reclaimed" && instance.Process == "stopped" && instance.Cleanup == "complete" {
			continue
		}
		report.Instances = append(report.Instances, nodev1.InventoryInstance{InstanceID: instance.InstanceID,
			Lifecycle: instance.Lifecycle, Process: instance.Process, Cleanup: instance.Cleanup, CurrentOperationID: instance.CurrentOperationID})
	}
	if report.Validate() != nil {
		report.Complete, report.Instances, report.ErrorCode = false, []nodev1.InventoryInstance{}, "CORE_LIST_INVALID"
	}
	return report
}
