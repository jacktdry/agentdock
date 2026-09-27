package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

const Version = "0.9.1"

var (
	Commit       string
	BuildDate    string
	Distribution = "uvwt/agentdock"
	UpdatePolicy = "apply"
)

type Info struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	BuildDate    string `json:"build_date"`
	Distribution string `json:"distribution"`
	UpdatePolicy string `json:"update_policy"`
	GoVersion    string `json:"go_version"`
	Platform     string `json:"platform"`
}

func Current() Info {
	info := Info{
		Version:      strings.TrimSpace(Version),
		Commit:       strings.TrimSpace(Commit),
		BuildDate:    strings.TrimSpace(BuildDate),
		Distribution: strings.TrimSpace(Distribution),
		UpdatePolicy: strings.TrimSpace(UpdatePolicy),
		GoVersion:    runtime.Version(),
		Platform:     runtime.GOOS + "/" + runtime.GOARCH,
	}
	if info.Distribution == "" {
		info.Distribution = "uvwt/agentdock"
	}
	if info.UpdatePolicy == "" {
		info.UpdatePolicy = "apply"
	}
	build, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = strings.TrimSpace(setting.Value)
				}
			case "vcs.time":
				if info.BuildDate == "" {
					info.BuildDate = strings.TrimSpace(setting.Value)
				}
			}
		}
	}
	if len(info.Commit) > 12 {
		info.Commit = info.Commit[:12]
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.BuildDate == "" {
		info.BuildDate = "unknown"
	}
	return info
}
