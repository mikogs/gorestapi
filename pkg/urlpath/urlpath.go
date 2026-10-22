package urlpath

import (
	"regexp"
	"strings"
)

var regexpID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ID returns the ID in the part of a URL path after the route, e.g. "123" or a UUID, and an empty string when there
// is no ID. It returns false for a path that is not a single segment of letters, digits, '-' and '_'. Whether the ID
// is valid for the record is checked when the record is read.
func ID(path string) (string, bool) {
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "", true
	}

	if !regexpID.MatchString(path) {
		return "", false
	}

	return path, true
}
