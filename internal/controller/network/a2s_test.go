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
	if !ProbeReadyInstances(context.Background(), []core.Instance{{Lifecycle: "active", Process: "running", Room: "ready", Port: port}}) {
		t.Fatal("live challenge query was not reported")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if ProbeReadyInstances(context.Background(), nil) {
		t.Fatal("idle node reported query success")
	}
	if ProbeReadyInstances(context.Background(), []core.Instance{{Lifecycle: "active", Process: "running", Room: "starting", Port: port}}) {
		t.Fatal("non-Ready instance reported query success")
	}
}

type errUnexpectedPacket struct{}

func (errUnexpectedPacket) Error() string { return "unexpected A2S packet" }
