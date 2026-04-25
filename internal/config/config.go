package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkyr/fig"
)

var configFilePath string

// Config holds the application configuration.
type Config struct {
	// MountPoint is the directory where the FUSE filesystem will be mounted.
	MountPoint string `fig:"mountpoint"`
	// RootScript is the path to the Lua script that defines routes.
	RootScript string `fig:"rootscript"`
	// Verbose enables debug logging.
	Verbose bool `fig:"verbose"`
	// Protocol selects the filesystem protocol: "fuse" (default) or "9p".
	Protocol string `fig:"protocol"`
	// NinepAddr is the TCP listen address for the 9p server (default "localhost:5640").
	NinepAddr string `fig:"ninepaddr"`
	// SSHAddr is the TCP listen address for the SSH server (default "localhost:5022").
	SSHAddr string `fig:"sshaddr"`
	// SSHAuthorizedKeys is the path to the authorized_keys file for SSH.
	SSHAuthorizedKeys string `fig:"sshauthorizedkeys"`
	// SSHHostKey is the path to the SSH host key file.
	SSHHostKey string `fig:"sshhostkey"`
}

// ConfigFileUsed returns the path to the config file that was loaded, if any.
func ConfigFileUsed() string {
	return configFilePath
}

// findConfigFile searches for a config file in the given directories.
// Returns the first found config file path, or empty string if none found.
func findConfigFile(dirs []string) string {
	extensions := []string{".toml", ".yaml", ".yml", ".json"}
	for _, dir := range dirs {
		for _, ext := range extensions {
			path := filepath.Join(dir, "config"+ext)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return ""
}

// LoadConfig loads configuration from config file, environment variables, and command line flags.
func LoadConfig() (*Config, error) {
	var cfg Config

	// Determine search directories
	dirs := []string{"."}
	if home := configHome(); home != "" {
		dirs = append(dirs, filepath.Join(home, "nklhd"))
	}
	dirs = append(dirs, "/etc/nklhd")
	for _, dir := range configDirs() {
		dirs = append(dirs, filepath.Join(dir, "nklhd"))
	}

	// Find config file
	configFile := findConfigFile(dirs)
	if configFile != "" {
		configFilePath = configFile
	}

	// Load config using fig with environment variable prefix NKLHD
	var opts []fig.Option
	if configFile != "" {
		// fig expects filename and directory separately
		dir := filepath.Dir(configFile)
		filename := filepath.Base(configFile)
		opts = append(opts, fig.File(filename), fig.Dirs(dir))
	}
	opts = append(opts, fig.Dirs(dirs...))
	opts = append(opts, fig.UseEnv("NKLHD"))

	err := fig.Load(&cfg, opts...)
	if err != nil {
		// If config file not found, that's okay - use defaults
		// Check if error is about missing config file
		errStr := err.Error()
		if strings.Contains(errStr, "file not found") || strings.Contains(errStr, "no such file") {
			log.Printf("note: no config file found; using defaults")
		} else {
			log.Printf("note: config file error: %v", err)
		}
	}

	// Resolve paths
	cfg.RootScript = ResolveScriptPath(cfg.RootScript)
	if cfg.MountPoint == "" {
		cfg.MountPoint = ResolveMountPoint("")
	}

	return &cfg, nil
}

// LoadConfigWithPath loads configuration with a specific config file path.
// If configPath ends with .lua or .tengo, it's treated as a script and no config file is loaded.
func LoadConfigWithPath(configPath string) (*Config, error) {
	if configPath == "" {
		return LoadConfig()
	}
	// If it's a Lua or Tengo script, set rootscript directly and skip config loading
	if strings.HasSuffix(configPath, ".lua") || strings.HasSuffix(configPath, ".tengo") {
		cfg := &Config{
			RootScript: ResolveScriptPath(configPath),
			MountPoint: ResolveMountPoint(""),
			Verbose:    false,
		}
		if cfg.MountPoint == "" {
			cfg.MountPoint = ResolveMountPoint("")
		}
		return cfg, nil
	}
	// Otherwise, load config from the specified file
	configFilePath = configPath
	var cfg Config
	// fig expects filename and directory separately for absolute paths
	dir := filepath.Dir(configPath)
	filename := filepath.Base(configPath)
	err := fig.Load(&cfg, fig.File(filename), fig.Dirs(dir), fig.UseEnv("NKLHD"))
	if err != nil {
		// If config file not found, that's an error
		return nil, err
	}
	// Resolve paths
	cfg.RootScript = ResolveScriptPath(cfg.RootScript)
	if cfg.MountPoint == "" {
		cfg.MountPoint = ResolveMountPoint("")
	}
	return &cfg, nil
}

// ResolveRootScript resolves the path to the root Lua script.
// If userPath is empty, searches XDG config directories.
func ResolveRootScript(userPath string) string {
	if userPath == "" {
		// Search for config in XDG_CONFIG_HOME and XDG_CONFIG_DIRS
		path, err := SearchConfigFile("nklhd/config.toml")
		if err == nil && path != "" {
			// Found existing config
			return path
		}
		// Not found, create in XDG_CONFIG_HOME
		path, err = ConfigFile("nklhd/config.toml")
		if err != nil {
			log.Printf("warning: could not get default config path: %v", err)
			return ""
		}
		return path
	}

	// If it's an absolute path or contains a path separator, treat as direct path
	if filepath.IsAbs(userPath) || strings.ContainsAny(userPath, "/\\") {
		return userPath
	}

	// Otherwise, treat as basename in XDG config home under nklhd subdirectory
	path, err := ConfigFile(filepath.Join("nklhd", userPath))
	if err != nil {
		log.Printf("warning: could not resolve config path: %v", err)
		return ""
	}
	return path
}

// ResolveMountPoint resolves the mount point path.
func ResolveMountPoint(userMount string) string {
	if userMount != "" {
		return userMount
	}
	// Default to runtime directory
	if RuntimeDir() == "" {
		log.Printf("warning: XDG_RUNTIME_DIR is not set")
		return ""
	}
	return filepath.Join(RuntimeDir(), "nklhd")
}

// ResolveScriptPath resolves the path to a Lua script, taking into account
// the config file location if available.
func ResolveScriptPath(userPath string) string {
	if userPath == "" {
		return ResolveRootScript("")
	}
	// If it's an absolute path or contains a path separator, treat as direct path
	if filepath.IsAbs(userPath) || strings.ContainsAny(userPath, "/\\") {
		return userPath
	}
	// Try to resolve relative to config file directory if config file was loaded
	if configFile := ConfigFileUsed(); configFile != "" {
		configDir := filepath.Dir(configFile)
		resolved := filepath.Join(configDir, userPath)
		if _, err := os.Stat(resolved); err == nil {
			return resolved
		}
		// If not found, fall through to XDG search
	}
	// Otherwise, treat as basename in XDG config home under nklhd subdirectory
	path, err := ConfigFile(filepath.Join("nklhd", userPath))
	if err != nil {
		log.Printf("warning: could not resolve script path: %v", err)
		return ""
	}
	return path
}

// InitViper initializes viper with config file search paths.
// configPath is the user-provided config file path (from --config flag).
// If empty, viper will search default locations.
// DEPRECATED: Use LoadConfigWithPath instead.
func InitViper(configPath string) error {
	// No-op, kept for compatibility
	return nil
}