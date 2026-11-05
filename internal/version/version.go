// Package version holds the release version. tagpr updates it.
package version

import (
	"runtime/debug"
	"strings"
)

// Version is the current release.
const Version = "0.5.1"

// String is the version with the commit it was built from, when the build
// knows it: a build in a clone does. go install does not, but it tells the
// version of the module installed, a tag or one naming the commit.
func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = &debug.BuildInfo{}
	}
	return format(Version, info)
}

func format(version string, info *debug.BuildInfo) string {
	rev := setting(info, "vcs.revision")
	if rev == "" {
		// Installed with go install, the module's version is known
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
		return version
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if setting(info, "vcs.modified") == "true" {
		rev += "-dirty"
	}
	return version + " (" + rev + ")"
}

func setting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
