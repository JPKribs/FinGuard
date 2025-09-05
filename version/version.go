package version

import (
	"strconv"
	"strings"
)

const (
	Version = "1.5.3"
)

// MARK: AsString
// Returns the version as a string
func AsString() string {
	return Version
}

// MARK: Major
// Returns the major version number as an integer
func Major() int {
	parts := strings.Split(Version, ".")
	if len(parts) == 0 {
		return 0
	}
	major, _ := strconv.Atoi(parts[0])
	return major
}

// MARK: Minor
// Returns the minor version number as an integer
func Minor() int {
	parts := strings.Split(Version, ".")
	if len(parts) < 2 {
		return 0
	}
	minor, _ := strconv.Atoi(parts[1])
	return minor
}

// MARK: Patch
// Returns the patch version number as an integer
func Patch() int {
	parts := strings.Split(Version, ".")
	if len(parts) < 3 {
		return 0
	}
	patch, _ := strconv.Atoi(parts[2])
	return patch
}
