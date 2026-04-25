// Package tests contains integration tests for nklhd.
package tests

import (
	"os"
	"strings"
	"testing"
)

// TestMain runs all tests and then performs a global cleanup of any leftover
// nklhd FUSE mounts that may have been left behind by interrupted tests.
func TestMain(m *testing.M) {
	code := m.Run()

	// Clean up any leftover nklhd FUSE mounts from /proc/mounts
	data, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		os.Exit(code)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.Contains(fields[0], "fuse") && strings.Contains(fields[1], "nklhd") {
			mountPoint := fields[1]
			unmountMountPoint(mountPoint)
		}
	}

	os.Exit(code)
}
