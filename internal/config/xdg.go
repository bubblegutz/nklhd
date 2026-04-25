package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// configHome returns the base directory for user-specific configuration files.
// Follows XDG Base Directory Specification with OS-specific fallbacks.
func configHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	case "windows":
		// Windows: use LOCALAPPDATA if set, else %USERPROFILE%\AppData\Local
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return v
		}
		return filepath.Join(home, "AppData", "Local")
	default:
		return filepath.Join(home, ".config")
	}
}

// configDirs returns the system configuration directories.
// Follows XDG_CONFIG_DIRS, defaulting to /etc/xdg.
func configDirs() []string {
	if v := os.Getenv("XDG_CONFIG_DIRS"); v != "" {
		return filepath.SplitList(v)
	}
	return []string{"/etc/xdg"}
}

// SearchConfigFile searches for a configuration file in XDG_CONFIG_HOME and XDG_CONFIG_DIRS.
// Returns the first found path, or an error if not found.
func SearchConfigFile(relPath string) (string, error) {
	// Search in config home first
	base := configHome()
	if base != "" {
		p := filepath.Join(base, relPath)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Search in config dirs
	for _, dir := range configDirs() {
		p := filepath.Join(dir, relPath)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

// ConfigFile returns the path to a configuration file in XDG_CONFIG_HOME.
// Does not create directories or check for existence.
func ConfigFile(relPath string) (string, error) {
	base := configHome()
	if base == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(base, relPath), nil
}

// RuntimeDir returns XDG_RUNTIME_DIR if set, otherwise a fallback directory.
func RuntimeDir() string {
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return v
	}
	// Fallback per OS
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "openbsd", "netbsd":
		// Try /run/user/<uid> on Linux
		if runtime.GOOS == "linux" {
			uid := os.Getuid()
			path := filepath.Join("/run", "user", fmt.Sprintf("%d", uid))
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		// Fallback to temp directory with user-specific subdirectory
		tmp := os.TempDir()
		uid := os.Getuid()
		return filepath.Join(tmp, fmt.Sprintf("xdg_runtime_%d", uid))
	case "windows":
		// Windows: use LOCALAPPDATA\Temp
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "Temp")
		}
		return os.TempDir()
	default:
		return os.TempDir()
	}
}