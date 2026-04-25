// Package router provides a robust pattern matching router based on go-chi/chi.
// It also defines shared types (TreeNode, NodeType) used by fuse and lua packages,
// replacing the previous interface isolation approach with direct package imports.
package router

import (
	"fmt"
)
// Request represents a request to a route handler.
type Request struct {
	Operation string
	Path      string
	Params    map[string]string
	Data      []byte
	Offset    int64
}

// Response represents a response from a route handler.
type Response struct {
	Data string
}
// ServeHandler is an optional interface that route handlers can implement.
type ServeHandler interface {
	Serve(*Request) (*Response, error)
}

// Handler is the interface for route handlers.
type Handler any



// Router maps operation + path pattern to handlers.
type Router struct {
	tree *node
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	return &Router{tree: &node{}}
}

// Register adds a handler for the given operation and path pattern.
func (r *Router) Register(op, pattern string, handler Handler) {

	if pattern == "" || pattern[0] != '/' {
		panic(fmt.Sprintf("router: routing pattern must begin with '/' in '%s'", pattern))
	}
	r.tree.InsertRoute(op, pattern, handler)
}

// Find finds a handler for the given operation and request path.
// Returns the handler, extracted path parameters, and the matched pattern.
func (r *Router) Find(op, path string) (Handler, map[string]string, string) {
	ctx := NewRouteContext()
	node, _, handler := r.tree.FindRoute(ctx, op, path)
	if node == nil || handler == nil {
		return nil, nil, ""
	}
	// Get the matched pattern
	pattern := ""
	if node.endpoints != nil && node.endpoints[op] != nil {
		pattern = node.endpoints[op].pattern
	}
	// Build params map from context
	params := make(map[string]string)
	for i, key := range ctx.URLParams.Keys {
		if i < len(ctx.URLParams.Values) {
			params[key] = ctx.URLParams.Values[i]
		}
	}
	return handler, params, pattern
}

// GetAllRoutePaths returns all unique route path patterns across all operations.
func (r *Router) GetAllRoutePaths() []string {
	routes := r.tree.routes()
	seen := make(map[string]bool)
	var paths []string
	for _, route := range routes {
		if !seen[route.Pattern] {
			seen[route.Pattern] = true
			paths = append(paths, route.Pattern)
		}
	}
	return paths
}

// Walk traverses all registered routes and calls fn for each operation-handler pair.
func (r *Router) Walk(fn func(op, pattern string, handler Handler) error) error {
	routes := r.tree.routes()
	for _, route := range routes {
		for op, handler := range route.Handlers {
			if err := fn(op, route.Pattern, handler); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetTree returns a hierarchical tree representation of all registered routes.
func (r *Router) GetTree() TreeNode {
	routes := r.tree.routes()

	handlers := make(map[string]map[string]any)
	for _, route := range routes {
		for op, handler := range route.Handlers {
			if handlers[op] == nil {
				handlers[op] = make(map[string]any)
			}
			handlers[op][route.Pattern] = handler
		}
	}

	return BuildTree(handlers)
}

// Serve processes a request by finding the appropriate handler and invoking it.
// If the handler implements ServeHandler, its Serve method is called.
// Otherwise, returns an error.
func (r *Router) Serve(req *Request) (*Response, error) {
	handler, params, _ := r.Find(req.Operation, req.Path)
	if handler == nil {
		return nil, fmt.Errorf("no handler registered for %s %s", req.Operation, req.Path)
	}
	// Merge request params with path params
	allParams := make(map[string]string)
	for k, v := range params {
		allParams[k] = v
	}
	for k, v := range req.Params {
		allParams[k] = v
	}
	// Update request with merged params
	req.Params = allParams
	
	// Try ServeHandler interface
	if sh, ok := handler.(ServeHandler); ok {
		return sh.Serve(req)
	}
	// Handler doesn't implement ServeHandler
	return nil, fmt.Errorf("handler does not implement ServeHandler")
}