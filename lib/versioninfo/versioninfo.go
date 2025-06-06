package versioninfo

import (
	"runtime/debug"
)

// version set at build-time
var version = "main"

// Version returns the version
func Version() string {
	return version
}

func CommitInfo() (hash string, timestamp string) {
	hash, timestamp = "unknown", "unknown"
	hashLen := 7

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return hash, timestamp
	}

	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			if len(s.Value) < hashLen {
				hashLen = len(s.Value)
			}
			hash = s.Value[:hashLen]
		} else if s.Key == "vcs.time" {
			timestamp = s.Value
		}
	}

	return hash, timestamp
}
