package ownership

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testNodeID = "11111111-2222-4333-8444-555555555555"

func TestOwnershipLockHelper(t *testing.T) {
	if os.Getenv("OWNERSHIP_TEST_CHILD") != "1" {
		return
	}
	lock, err := Acquire(os.Getenv("OWNERSHIP_TEST_DATA_DIR"), testNodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fmt.Println("LOCKED")
	_, _ = bufio.NewReader(os.Stdin).ReadByte()
}

func TestExclusiveOwnershipAndCrashRelease(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "core-data")
	if err := os.Mkdir(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := Acquire(dataDir, testNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(dataDir, testNodeID); !errors.Is(err, ErrHeld) {
		t.Fatalf("duplicate owner acquired: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnershipLockHelper$")
	cmd.Env = append(os.Environ(), "OWNERSHIP_TEST_CHILD=1", "OWNERSHIP_TEST_DATA_DIR="+dataDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "LOCKED" {
		t.Fatalf("child ownership failed: %q %v", line, err)
	}
	if _, err := Acquire(dataDir, testNodeID); !errors.Is(err, ErrHeld) {
		t.Fatalf("parent bypassed child owner: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for {
		recovered, err := Acquire(dataDir, testNodeID)
		if err == nil {
			_ = recovered.Close()
			break
		}
		if !errors.Is(err, ErrHeld) || time.Now().After(deadline) {
			t.Fatalf("OS did not release dead process ownership: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
