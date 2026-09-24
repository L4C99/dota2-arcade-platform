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
	config := httpapi.Config{PublicOrigin: os.Getenv("PLATFORM_PUBLIC_ORIGIN"), Development: environment == "development"}
	handler, err := httpapi.NewHandler(s, config)
	if err != nil {
		return err
	}
	address := os.Getenv("PLATFORM_LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	if config.Development {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return errors.New("development listener must bind to loopback")
		}
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
