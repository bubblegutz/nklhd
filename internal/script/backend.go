package script

import "github.com/bubblegutz/nklhd/internal/router"

// Backend is the interface that scripting language executors must implement
// to provide routing capabilities to nklhd.
type Backend interface {
	// LoadScript loads and executes a script file.
	LoadScript(path string) error

	// Close cleans up all resources held by the executor.
	Close()

	// GetRouter returns the underlying router instance for use by servers.
	GetRouter() *router.Router
}
