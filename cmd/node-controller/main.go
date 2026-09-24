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
	"github.com/L4C99/dota2-arcade-platform/internal/controller/config"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/platformclient"
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
	send := func(ctx context.Context) (string, error) {
		facts := conf.Facts(buildinfo.Version, 0)
		result, err := client.Heartbeat(ctx, facts)
		return result.CompatibilityStatus, err
	}
	if args[0] == "heartbeat" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		status, err := send(ctx)
		if err != nil {
			return err
		}
		fmt.Println("heartbeat accepted; compatibility=" + status)
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	lastStatus := ""
	for {
		status, err := send(ctx)
		if err != nil {
			log.Printf("heartbeat failed: %v", err)
		} else if status != lastStatus {
			log.Printf("node compatibility: %s", status)
			lastStatus = status
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
