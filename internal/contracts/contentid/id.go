// Package contentid defines IDs shared by catalog registration and Content Tool.
package contentid

import (
	"regexp"
	"strings"
)

var workshop = regexp.MustCompile(`^[0-9]{1,20}$`)
var version = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func ValidWorkshop(id string) bool { return workshop.MatchString(id) }
func ValidVersion(id string) bool {
	return version.MatchString(id) && !strings.EqualFold(id, "current") && !strings.EqualFold(id, "previous") && !strings.EqualFold(id, "pending")
}
