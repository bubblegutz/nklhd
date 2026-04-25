// Package app defines the language-agnostic API for nklhd's app.* interface.
// This allows different embedded languages (Lua, Lisp, etc.) to expose the same API.
package app

import (
	"os"
	"strings"
)

// Handler represents a generic handler function in any embedded language.
// The actual type is language-specific, but adapters will convert it to a router.ServeHandler.
type Handler any

// Backend is the interface that language adapters must implement to provide
// app.* API functionality to their scripting environment.
type Backend interface {
	// RegisterRead registers a handler for read operations (FUSE read).
	RegisterRead(pathPattern string, handler Handler) error

	// RegisterWrite registers a handler for write operations (FUSE write).
	RegisterWrite(pathPattern string, handler Handler) error

	// RegisterCreate registers a handler for create operations (FUSE create).
	RegisterCreate(pathPattern string, handler Handler) error

	// RegisterUnlink registers a handler for remove operations (FUSE remove/unlink).
	RegisterUnlink(pathPattern string, handler Handler) error

	// RegisterGetAttr registers a handler for stat operations (FUSE getattr).
	RegisterGetAttr(pathPattern string, handler Handler) error

	// RegisterLookup registers a handler for walk operations (custom traversal).
	RegisterLookup(pathPattern string, handler Handler) error

	// RegisterOpen registers a handler for open operations (FUSE open).
	RegisterOpen(pathPattern string, handler Handler) error

	// RegisterReadDir registers a handler for directory listing operations (FUSE readdir).
	RegisterReadDir(pathPattern string, handler Handler) error

	// Environment returns a map of environment variables to expose as app.env.
	Environment() map[string]string

	// Log logs a message at info level.
	Log(msg string)

	// Debug logs a message at debug level (only if debug logging is enabled).
	Debug(msg string)
}

// EnvironmentFromOS returns environment variables from os.Environ() as a map.
func EnvironmentFromOS() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		key, val, ok := strings.Cut(e, "=")
		if ok {
			env[key] = val
		}
	}
	return env
}
