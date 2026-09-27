package network

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

func TestA2SChallengeAndReadyFact(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	port := server.LocalAddr().(*net.UDPAddr).Port
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 128)
		for step := 0; step < 2; step++ {
			_ = server.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, addr, err := server.ReadFromUDP(buf)
			if err != nil {
				done <- err
				return
			}
			if n < len(a2sInfoRequest) || !bytes.Equal(buf[:len(a2sInfoRequest)], a2sInfoRequest) {
				done <- errUnexpectedPacket{}
				return
			}
			if step == 0 {
				_, err = server.WriteToUDP([]byte{0xff, 0xff, 0xff, 0xff, 0x41, 1, 2, 3, 4}, addr)
			} else {
				if !bytes.Equal(buf[len(a2sInfoRequest):n], []byte{1, 2, 3, 4}) {
					done <- errUnexpectedPacket{}
					return
				}
				_, err = server.WriteToUDP([]byte{0xff, 0xff, 0xff, 0xff, 0x49, 0x11, 0x00, 0x00}, addr)
			}
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	diagnostics := ProbeReadyInstances(context.Background(), []core.Instance{{InstanceID: "ready-a", Lifecycle: "active", Process: "running", Room: "ready", Port: port}})
	if len(diagnostics) != 1 || diagnostics[0].Status != "ok" || diagnostics[0].InstanceID != "ready-a" || diagnostics[0].CheckedAt == "" {
		t.Fatal("live challenge query was not reported")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(ProbeReadyInstances(context.Background(), nil)) != 0 {
		t.Fatal("idle node reported query success")
	}
	if len(ProbeReadyInstances(context.Background(), []core.Instance{{Lifecycle: "active", Process: "running", Room: "starting", Port: port}})) != 0 {
		t.Fatal("non-Ready instance reported query success")
	}
}

func TestReadyInstancesHaveIndependentA2SFacts(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	port := server.LocalAddr().(*net.UDPAddr).Port
	go func() {
		buf := make([]byte, 128)
		_ = server.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, addr, err := server.ReadFromUDP(buf)
		if err == nil {
			_, _ = server.WriteToUDP([]byte{0xff, 0xff, 0xff, 0xff, 0x49, 0x11, 0x00, 0x00}, addr)
		}
	}()
	closed, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	failedPort := closed.LocalAddr().(*net.UDPAddr).Port
	_ = closed.Close()
	diagnostics := ProbeReadyInstances(context.Background(), []core.Instance{
		{InstanceID: "a", Lifecycle: "active", Process: "running", Room: "ready", Port: port},
		{InstanceID: "b", Lifecycle: "active", Process: "running", Room: "ready", Port: failedPort},
	})
	if len(diagnostics) != 2 || diagnostics[0].Status != "ok" || diagnostics[1].Status != "failed" || diagnostics[1].InstanceID != "b" {
		t.Fatalf("expected independent ok/failed facts, got %+v", diagnostics)
	}
}

type errUnexpectedPacket struct{}

func (errUnexpectedPacket) Error() string { return "unexpected A2S packet" }

func TestA4ProbeBudgetSkipsUnqueriedInstances(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	instances := make([]core.Instance, 100)
	for i := range instances {
		instances[i] = core.Instance{InstanceID: "silent", Lifecycle: "active", Process: "running", Room: "ready", Port: server.LocalAddr().(*net.UDPAddr).Port}
	}
	started := time.Now()
	facts := ProbeReadyInstances(context.Background(), instances)
	if time.Since(started) > 2500*time.Millisecond || len(facts) != 2 {
		t.Fatalf("unbounded probes or manufactured failures: %d %v", len(facts), time.Since(started))
	}
	for _, fact := range facts {
		if fact.Status != "failed" {
			t.Fatal(fact)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if facts := ProbeReadyInstances(ctx, instances); len(facts) != 0 {
		t.Fatal("cancelled budget manufactured failures")
	}
}
