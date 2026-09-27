package a4proof

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

// This is a deterministic scheduling witness, not a real IPC/Dota test.
// The barrier models a request goroutine paused after readLine and before
// handler/Respond. No d2core source is changed and no process is launched.
func TestSameIncarnationRejectionDoesNotFenceEarlierRequest(t *testing.T) {
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	template := config.Template{SchemaVersion: 1, Name: "proof", Executable: exe, WorkingDirectory: dir,
		Arguments: []string{"-port", "{{game_port}}", "-con_logfile", "{{log_path}}", "+exec", "{{cfg_name}}"},
		CFG:       config.CFGConfig{Directory: dir, Lines: []string{`hostname "{{instance_id}}"`}}, Readiness: config.Readiness{SuccessAll: []string{"ready"}}}
	b, err := json.Marshal(template)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "template.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := records.Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	ports := records.PortRange{Min: port, Max: port}
	var managerMutex sync.Mutex
	delayed := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	key := "same-frozen-key"
	go func() {
		close(delayed)
		<-release
		managerMutex.Lock()
		defer managerMutex.Unlock()
		in, op, err := store.CreateChecked(path, 0, key, ports, nil)
		if err == nil && (in == nil || op == nil) {
			err = errors.New("missing persisted IDs")
		}
		done <- err
	}()
	<-delayed
	// The client deadline expires while its already-read server work is paused.
	select {
	case <-done:
		t.Fatal("first request unexpectedly finished")
	case <-time.After(10 * time.Millisecond):
	}
	managerMutex.Lock()
	_, _, retryErr := store.CreateChecked(path, 0, key, ports, nil)
	managerMutex.Unlock()
	var rejection *records.Failure
	if !errors.As(retryErr, &rejection) || rejection.Code != "NO_PORT_AVAILABLE" || rejection.Stage != "validate" || rejection.InstanceID != "" || rejection.OperationID != "" {
		t.Fatalf("retry: %v", retryErr)
	}
	if len(store.State.Keys) != 0 {
		t.Fatal("key unexpectedly existed at rejection")
	}
	t.Log("same store, same frozen key/request: retry returned NO_PORT_AVAILABLE@validate without IDs")
	// The proposed rule would release here. A resource becomes free before
	// the original request finally enters the same manager's critical section.
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(store.State.Keys) != 1 || len(store.State.Instances) != 1 || len(store.State.Operations) != 1 {
		t.Fatal("late original request did not persist")
	}
	reopened, err := records.Open(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.State.Keys) != 1 || len(reopened.State.Instances) != 1 {
		t.Fatal("late side effect was not durable")
	}
	t.Log("after that rejection, delayed original request persisted instance + operation + key; same Store, no restart")
}
