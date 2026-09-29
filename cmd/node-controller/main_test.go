package main

import (
	"errors"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/controller/core"
)

func TestInventoryForListFailClosed(t *testing.T) {
	complete := inventoryForList("one", core.ListResult{Instances: []core.Instance{}}, nil)
	if !complete.Complete || len(complete.Instances) != 0 || complete.ErrorCode != "" {
		t.Fatalf("successful empty List: %+v", complete)
	}
	failed := inventoryForList("two", core.ListResult{}, errors.New("transport lost"))
	if failed.Complete || len(failed.Instances) != 0 || failed.ErrorCode != "CORE_LIST_FAILED" {
		t.Fatalf("failed List looked complete: %+v", failed)
	}
	invalid := inventoryForList("three", core.ListResult{Instances: []core.Instance{{InstanceID: "i"}}}, nil)
	if invalid.Complete || len(invalid.Instances) != 0 || invalid.ErrorCode != "CORE_LIST_INVALID" {
		t.Fatalf("invalid instance looked complete: %+v", invalid)
	}
	terminal := inventoryForList("four", core.ListResult{Instances: []core.Instance{{InstanceID: "i", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}}}, nil)
	if !terminal.Complete || len(terminal.Instances) != 0 {
		t.Fatalf("reclaimed instance remained active: %+v", terminal)
	}
}
