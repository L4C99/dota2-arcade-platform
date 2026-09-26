package main

import (
	"context"
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
	if len(args) == 1 && args[0] == "version" {
		fmt.Printf("node-controller %s (%s)\n", buildinfo.Version, buildinfo.Commit())
		return nil
	}
	if len(args) != 3 || args[1] != "--config" || (args[0] != "heartbeat" && args[0] != "run") {
		return errors.New("usage: node-controller heartbeat|run --config ABS_PATH")
	}
	conf, secret, err := config.Load(args[2])
	if err != nil {
		return err
	}
	client := platformclient.New(conf.PlatformURL, conf.NodeID, secret)
	coreClient, err := core.New(conf.D2CoreDataDir)
	if err != nil {
		return err
	}
	worker := &runner.Runner{Platform: client, Core: coreClient, TemplateBindings: conf.TemplateBindings, Network: conf.Network}
	factReader := config.NewFactReader(conf)
	send := func(ctx context.Context) (nodev1.HeartbeatResult, error) {
		protocol := 0
		list, listErr := coreClient.List(ctx)
		if listErr == nil {
			protocol = 1
		}
		facts := factReader.Facts(buildinfo.Version, protocol)
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
		return result, err
	}
	if args[0] == "heartbeat" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		result, err := send(ctx)
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
		result, err := send(cycle)
		status := result.CompatibilityStatus
		if err != nil {
			log.Printf("heartbeat failed: %v", err)
		} else if status != lastStatus {
			log.Printf("node compatibility: %s", status)
			lastStatus = status
		}
		if err == nil && status == "compatible" {
			if err := worker.Step(cycle); err != nil {
				log.Printf("node job cycle failed: %v", err)
			} else if result.ReconcileRequestedGeneration > result.ReconcileCompletedGeneration {
				if err := client.CompleteReconcile(cycle, result.ReconcileRequestedGeneration); err != nil {
					log.Printf("node reconcile acknowledgement failed: %v", err)
				}
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
