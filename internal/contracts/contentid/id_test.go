package contentid

import (
	"strings"
	"testing"
)

func TestIDBoundaries(t *testing.T) {
	for _, id := range []string{"1", strings.Repeat("9", 20)} {
		if !ValidWorkshop(id) {
			t.Fatal(id)
		}
	}
	for _, id := range []string{"", strings.Repeat("9", 21), "../1", "a", "１２"} {
		if ValidWorkshop(id) {
			t.Fatal(id)
		}
	}
	for _, id := range []string{"v1", "A.b_2-3", strings.Repeat("a", 128)} {
		if !ValidVersion(id) {
			t.Fatal(id)
		}
	}
	for _, id := range []string{"", "CURRENT", "Previous", "pending", "../v", "a/b", "a b", strings.Repeat("a", 129)} {
		if ValidVersion(id) {
			t.Fatal(id)
		}
	}
}
