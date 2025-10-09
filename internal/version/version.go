package version

import (
	"fmt"
	"runtime"
)

// Build information - set via ldflags during build
var (
	Version    = "dev"
	BuildTime  = "unknown"
	CommitHash = "unknown"
)

// Info contains version information
type Info struct {
	Version    string `json:"version"`
	BuildTime  string `json:"build_time"`
	CommitHash string `json:"commit_hash"`
	GoVersion  string `json:"go_version"`
	Platform   string `json:"platform"`
}

// GetInfo returns version information
func GetInfo() Info {
	return Info{
		Version:    Version,
		BuildTime:  BuildTime,
		CommitHash: CommitHash,
		GoVersion:  runtime.Version(),
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf("%s (built %s from %s on %s with %s)",
		i.Version, i.BuildTime, i.CommitHash, i.Platform, i.GoVersion)
}

// Short returns a short version string
func (i Info) Short() string {
	return i.Version
}