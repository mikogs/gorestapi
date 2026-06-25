package urlpath

import (
	"regexp"
	"strings"
)

var (
	regexpNumeric = regexp.MustCompile(`^[0-9]+$`)
)

func ID(path string) (string, bool) {
	// path can start and end with /
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "", true
	}

	// take the last part of the path
	pathParts := strings.Split(path, "/")
	path = pathParts[len(pathParts)-1]

	isNumeric := regexpNumeric.MatchString(path)
	if !isNumeric {
		return "", false
	}

	return path, true
}
