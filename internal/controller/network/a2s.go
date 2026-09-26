package network

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

var a2sHeader = []byte{0xff, 0xff, 0xff, 0xff}
var a2sInfoRequest = append(append([]byte{}, a2sHeader...), append([]byte{0x54}, []byte("Source Engine Query\x00")...)...)

// QueryA2SInfo checks one local game port. Challenge negotiation is supported;
// a successful query is only a diagnostic, never proof of real client entry.
func QueryA2SInfo(ctx context.Context, port int) error {
	if port < 1 || port > 65535 {
		return errors.New("invalid A2S port")
	}
	dialer := net.Dialer{Timeout: 350 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(700 * time.Millisecond)
	if when, ok := ctx.Deadline(); ok && when.Before(deadline) {
		deadline = when
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	request := a2sInfoRequest
	buf := make([]byte, 2048)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := conn.Write(request); err != nil {
			return err
		}
		n, err := conn.Read(buf)
		if err != nil {
			return err
		}
		if n < 5 || !bytes.Equal(buf[:4], a2sHeader) {
			return errors.New("invalid A2S response header")
		}
		switch buf[4] {
		case 0x49:
			if n < 8 {
				return errors.New("incomplete A2S info response")
			}
			return nil
		case 0x41:
			if attempt != 0 || n != 9 {
				return errors.New("invalid A2S challenge")
			}
			request = append(append([]byte{}, a2sInfoRequest...), buf[5:9]...)
		default:
			return errors.New("unexpected A2S response")
		}
	}
	return errors.New("A2S info response absent")
}

// ProbeReadyInstances records an independent diagnostic for every Ready server.
// An empty slice means there is no live query fact, not a query failure.
func ProbeReadyInstances(ctx context.Context, instances []core.Instance) []nodev1.A2SDiagnostic {
	diagnostics := make([]nodev1.A2SDiagnostic, 0)
	for _, instance := range instances {
		if instance.Lifecycle != "active" || instance.Process != "running" || instance.Room != "ready" {
			continue
		}
		status := "ok"
		if QueryA2SInfo(ctx, instance.Port) != nil {
			status = "failed"
		}
		diagnostics = append(diagnostics, nodev1.A2SDiagnostic{InstanceID: instance.InstanceID, LocalPort: instance.Port,
			Status: status, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	return diagnostics
}
