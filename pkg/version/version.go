package version

import (
	"strings"
)

var (
	// Version contains application version
	Version string
	// BuildTime contains application build time
	BuildTime string
	// GoVersion contains the golang version uses to build this binary
	GoVersion string
)

type printer interface {
	Printf(format string, i ...interface{})
}

// Show displays various version information
func Show(out printer) {
	out.Printf("Version:\t%s\n", Version)
	out.Printf("%s\n", strings.ReplaceAll(GoVersion, "go version go", "Golang     :\t"))
	out.Printf("Build Time :\t%s\n", BuildTime)
}
