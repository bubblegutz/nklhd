// Package script provides shared route registry functionality used by
// language-specific executors (Lua, Tengo, etc.).
package script

import (
	"strconv"
	"strings"

	"github.com/bubblegutz/nklhd/internal/router"
)

// Registry stores registered routes by operation and path pattern.
type Registry struct {
	router            *router.Router
	handlers          map[string]map[string]router.ServeHandler
	convertedPatterns map[string]string
	paramMappings     map[string]map[string]string
}

// NewRegistry creates a new empty route registry.
func NewRegistry() *Registry {
	return &Registry{
		router:            router.NewRouter(),
		handlers:          make(map[string]map[string]router.ServeHandler),
		convertedPatterns: make(map[string]string),
		paramMappings:     make(map[string]map[string]string),
	}
}

// Register adds a route handler for the given operation and path pattern.
func (r *Registry) Register(operation, pathPattern string, handler router.ServeHandler) {
	if r.handlers[operation] == nil {
		r.handlers[operation] = make(map[string]router.ServeHandler)
	}
	r.handlers[operation][pathPattern] = handler

	// Convert glob patterns to chi-compatible regex patterns
	convertedPattern, paramMap := router.ConvertGlobPattern(pathPattern)

	r.convertedPatterns[convertedPattern] = pathPattern
	if len(paramMap) > 0 {
		r.paramMappings[pathPattern] = paramMap
	}

	// Register with the underlying router using converted pattern
	r.router.Register(operation, convertedPattern, handler)
}

// FindHandler finds a handler for the given operation and request path.
// Returns the handler and extracted path parameters.
func (r *Registry) FindHandler(operation, requestPath string) (router.ServeHandler, map[string]string) {
	handler, params, _ := r.router.Find(operation, requestPath)
	if handler == nil {
		return nil, nil
	}
	// Cast to ServeHandler
	sh, ok := handler.(router.ServeHandler)
	if !ok {
		return nil, nil
	}

	// Process params to map generated keys to original keys
	mappedParams := make(map[string]string)
	wildcardCount := 0

	for key, value := range params {
		// Check if this is a generated glob param (_globN) - drop it
		if strings.HasPrefix(key, "_glob") {
			continue
		}

		// Check if this is a generated wildcard param (_wildcardN)
		if strings.HasPrefix(key, "_wildcard") {
			// Determine wildcard index
			idx := 0
			if len(key) > 9 {
				// Extract number after "_wildcard"
				if n, err := strconv.Atoi(key[9:]); err == nil {
					idx = n
				}
			}
			wildcardCount++

			// Map to traditional wildcard param names
			if idx == 0 {
				// First wildcard: also set "*" for backward compatibility
				mappedParams["*"] = value
				mappedParams["*1"] = value
			} else {
				// Subsequent wildcards: "*2", "*3", etc.
				mappedParams["*"+strconv.Itoa(idx+1)] = value
			}
			continue
		}

		// Regular param (e.g., {id}) - keep as is
		mappedParams[key] = value
	}

	return sh, mappedParams
}

// GetAllRoutePaths returns all unique route path patterns across all operations.
func (r *Registry) GetAllRoutePaths() []string {
	return r.router.GetAllRoutePaths()
}

// GetRouter returns the underlying router instance.
func (r *Registry) GetRouter() *router.Router {
	return r.router
}

// BuildTree constructs a hierarchical tree from all registered routes.
func (r *Registry) BuildTree() router.TreeNode {
	// Convert handlers to interface{} map
	conv := make(map[string]map[string]any)
	for op, opRoutes := range r.handlers {
		conv[op] = make(map[string]any)
		for pattern, handler := range opRoutes {
			conv[op][pattern] = handler
		}
	}
	return router.BuildTree(conv)
}
