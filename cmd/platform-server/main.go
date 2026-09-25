package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/buildinfo"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/httpapi"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "version" {
		fmt.Printf("platform-server %s (%s)\n", buildinfo.Version, buildinfo.Commit())
		return nil
	}
	if len(args) == 0 {
		return errors.New("usage: platform-server serve|migrate|admin create <username>|admin reset-password <username>|version")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := store.Open(ctx, os.Getenv("PLATFORM_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.ApplyMigrations(ctx); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	switch args[0] {
	case "migrate":
		if len(args) != 1 {
			return errors.New("usage: platform-server migrate")
		}
		fmt.Println("migrations current")
		return nil
	case "admin":
		if len(args) != 3 {
			return errors.New("usage: platform-server admin create|reset-password <username> (password from stdin)")
		}
		password, err := readPasswordFromPipe()
		if err != nil {
			return err
		}
		switch args[1] {
		case "create":
			_, err = s.CreateAdmin(ctx, args[2], password)
		case "reset-password":
			err = s.ResetAdminPassword(ctx, args[2], password)
		default:
			return errors.New("unknown admin command")
		}
		if err != nil {
			return err
		}
		fmt.Println("admin account updated")
		return nil
	case "node":
		if len(args) == 3 && (args[1] == "drain" || args[1] == "resume") {
			if err := s.SetNodeDrain(ctx, args[2], args[1] == "drain"); err != nil {
				return err
			}
			draining, err := s.NodeDraining(ctx, args[2])
			if err != nil {
				return err
			}
			fmt.Printf("node_id=%s draining=%t\n", args[2], draining)
			return nil
		}
		if (len(args) == 3 || len(args) == 4) && args[1] == "priority" {
			if len(args) == 4 {
				priority, err := strconv.Atoi(args[3])
				if err != nil {
					return errors.New("node priority must be an integer")
				}
				if err := s.SetNodePriority(ctx, args[2], priority); err != nil {
					return err
				}
			}
			priority, err := s.NodePriority(ctx, args[2])
			if err != nil {
				return err
			}
			fmt.Printf("node_id=%s priority=%d\n", args[2], priority)
			return nil
		}
		if (len(args) == 3 || len(args) == 4) && args[1] == "capacity" {
			if len(args) == 4 {
				desired, err := strconv.Atoi(args[3])
				if err != nil {
					return errors.New("desired capacity must be an integer")
				}
				if err := s.SetDesiredCapacity(ctx, args[2], desired); err != nil {
					return err
				}
			}
			capacity, err := s.Capacity(ctx, args[2])
			if err != nil {
				return err
			}
			fmt.Printf("node_id=%s hard=%d desired=%d occupied=%d effective=%d\n",
				capacity.NodeID, capacity.Hard, capacity.Desired, capacity.Occupied, min(capacity.Hard, capacity.Desired))
			return nil
		}
		if len(args) == 4 && args[1] == "register" {
			id, secret, err := s.RegisterNode(ctx, args[2], args[3])
			if err != nil {
				return err
			}
			fmt.Printf("node_id=%s\nnode_secret=%s\n", id, secret)
			return nil
		}
		if len(args) >= 4 && args[1] == "integration-job" {
			var id string
			var err error
			switch args[3] {
			case "create":
				if len(args) != 5 && len(args) != 6 {
					return errors.New("create requires template binding and optional port")
				}
				port := 0
				if len(args) == 6 {
					port, err = strconv.Atoi(args[5])
					if err != nil {
						return err
					}
				}
				id, err = s.CreateIntegrationCreateJob(ctx, args[2], args[4], port)
			case "stop":
				if len(args) != 5 {
					return errors.New("stop requires instance ID")
				}
				id, err = s.CreateIntegrationJob(ctx, args[2], "stop", args[4])
			default:
				return errors.New("invalid integration job kind")
			}
			if err != nil {
				return err
			}
			fmt.Printf("node_job_id=%s\n", id)
			return nil
		}
		return errors.New("usage: platform-server node register <name> <windows|linux> | node capacity <node-id> [desired] | node priority <node-id> [priority] | node drain|resume <node-id> | node integration-job <node-id> create <template-binding> [port] | node integration-job <node-id> stop <instance-id>")
	case "serve":
		if len(args) != 1 {
			return errors.New("usage: platform-server serve")
		}
		return serve(s)
	default:
		return errors.New("unknown command")
	}
}

func readPasswordFromPipe() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", errors.New("pipe the admin password on stdin; it is not accepted as an argument")
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1026))
	if err != nil {
		return "", err
	}
	if len(data) > 1025 {
		return "", errors.New("admin password too long")
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"), nil
}

func serve(s *store.Store) error {
	environment := os.Getenv("PLATFORM_ENV")
	if environment != "" && environment != "production" && environment != "development" {
		return errors.New("PLATFORM_ENV must be production or development")
	}
	partySize, err := strconv.Atoi(os.Getenv("PLATFORM_MAX_PARTY_SIZE"))
	if err != nil || partySize <= 0 {
		return errors.New("PLATFORM_MAX_PARTY_SIZE must be explicitly set to a positive integer")
	}
	configCtx, configDone := context.WithTimeout(context.Background(), 5*time.Second)
	err = s.ConfigurePartySize(configCtx, partySize)
	configDone()
	if err != nil {
		return fmt.Errorf("configure max_party_size: %w", err)
	}
	config := httpapi.Config{PublicOrigin: os.Getenv("PLATFORM_PUBLIC_ORIGIN"), Development: environment == "development",
		WebRoot: os.Getenv("PLATFORM_WEB_ROOT")}
	handler, err := httpapi.NewHandler(s, config)
	if err != nil {
		return err
	}
	address := os.Getenv("PLATFORM_LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("platform-server must bind to loopback behind the HTTPS proxy")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			cycle, done := context.WithTimeout(stop, 5*time.Second)
			_, err := s.TryAllocateOne(cycle)
			done()
			if err != nil && stop.Err() == nil {
				log.Printf("allocation cycle: %v", err)
			}
			select {
			case <-stop.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	go func() {
		<-stop.Done()
		ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = server.Shutdown(ctx)
	}()
	log.Printf("platform-server listening on %s", listener.Addr())
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
