package store

import (
	"context"
	"errors"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"testing"
	"time"
)

func TestA4JoinFactRepairsTerminalCreate(t *testing.T) {
	s, node, _, _, allocation := p4cRunning(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `UPDATE allocations SET join_local_port=NULL,join_public_port=NULL,join_connect_host=NULL,join_protocol_ip=NULL,join_entry_config_revision=NULL,join_info_available_at=NULL,join_info_error_code='PORT_MAPPING_UNAVAILABLE' WHERE id=$1`, allocation); err != nil {
		t.Fatal(err)
	}
	f := nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "active", Lifecycle: "active", Process: "running", Room: "ready", Port: 28000, JoinInfo: &nodev1.JoinInfo{LocalPort: 28000, PublicPort: 28000, ConnectHost: "127.0.0.1", EntryConfigRevision: nodev1.EntryConfigRevision(p1TestHeartbeat("test-v1").Network)}}
	for _, bad := range []string{"identity", "room", "revision"} {
		copy := f
		j := *f.JoinInfo
		copy.JoinInfo = &j
		switch bad {
		case "identity":
			copy.InstanceID = "wrong"
		case "room":
			copy.Room = "loading"
		case "revision":
			j.EntryConfigRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}
		if err := s.ReportInstanceFact(ctx, node, allocation, copy); !errors.Is(err, ErrJobConflict) {
			t.Fatalf("%s: %v", bad, err)
		}
	}
	if err := s.ReportInstanceFact(ctx, node, allocation, f); err != nil {
		t.Fatal(err)
	}
	var first, again time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT join_info_available_at FROM allocations WHERE id=$1`, allocation).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := s.ReportInstanceFact(ctx, node, allocation, f); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT join_info_available_at FROM allocations WHERE id=$1`, allocation).Scan(&again); err != nil || !first.Equal(again) {
		t.Fatalf("first timestamp changed %v", err)
	}
}
