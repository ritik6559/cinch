package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func init() {
	if Commit != "" {
		return // the linked already gave us real values
	}

	info, ok := debug.ReadBuildInfo() // go stores some git information while building
	if !ok {
		return // not built as a module, nothing to use
	}

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			Commit = setting.Value
		case "vcs.time":
			Date = setting.Value
		}
	}
}

func Short() string {
	return Version
}

func Detailed() string {
	var b strings.Builder

	fmt.Fprintf(&b, "cinch %s", Version)
	if Commit != "" {
		fmt.Fprintf(&b, " (%s", shortCommit())
		if Date != "" {
			fmt.Fprintf(&b, ", %s", Date)
		}
		b.WriteString(")")
	}
	fmt.Fprintf(&b, "\n%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	return b.String()
}

// UserAgent identifies cinch in HTTP requests to the provider.
func UserAgent() string { return "cinch/" + Version }

func shortCommit() string {
	if len(Commit) > 7 {
		return Commit[:7]
	}
	return Commit
}
