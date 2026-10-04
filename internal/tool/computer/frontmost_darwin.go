//go:build darwin

package computer

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
)

var (
	frontNamePattern   = regexp.MustCompile(`(?m)^"([^"]+)"`)
	frontBundlePattern = regexp.MustCompile(`(?m)bundleID="([^"]+)"`)
	frontPIDPattern    = regexp.MustCompile(`(?m)pid = ([0-9]+)`)
)

func platformFrontmostApp(ctx context.Context) (*AppIdentity, error) {
	front, err := exec.CommandContext(ctx, "lsappinfo", "front").Output()
	if err != nil {
		return nil, err
	}
	info, err := exec.CommandContext(ctx, "lsappinfo", "info", "-only", "bundleID", "-only", "name", "-only", "pid", string(front)).Output()
	if err != nil {
		return nil, err
	}
	result := &AppIdentity{}
	if match := frontNamePattern.FindSubmatch(info); len(match) == 2 {
		result.Name = string(match[1])
	}
	if match := frontBundlePattern.FindSubmatch(info); len(match) == 2 {
		result.BundleID = string(match[1])
	}
	if match := frontPIDPattern.FindSubmatch(info); len(match) == 2 {
		result.PID, _ = strconv.Atoi(string(match[1]))
	}
	if result.Name == "" && result.BundleID == "" && result.PID == 0 {
		return nil, nil
	}
	return result, nil
}
