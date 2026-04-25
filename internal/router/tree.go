// Package router provides hierarchical tree representation of routes.
package router

import (
	"regexp"
	"strings"
)

// NodeType represents the type of a node in the route tree.
type NodeType int

const (
	NodeTypeDir NodeType = iota
	NodeTypeFile
	NodeTypeParam
	NodeTypeWildcard
	NodeTypeGlob
)

// TreeNode defines the interface for tree nodes used by frontends for navigation.
type TreeNode interface {
	// GetName returns the component name of this node.
	GetName() string
	// GetType returns the type of this node.
	GetType() NodeType
	// GetPattern returns the full route pattern for leaf nodes (empty for non-leaf nodes).
	GetPattern() string
	// GetChildren returns the child nodes of this node.
	GetChildren() map[string]TreeNode
	// GetHandlers returns the operation handlers for this node.
	GetHandlers() map[string]any
	// FindNode finds a node in the tree matching the given path components.
	FindNode(components []string) (TreeNode, map[string]string)
	// ListChildren returns the names of immediate children of this node.
	ListChildren() []string
	// GetHandler returns the handler for a given operation, if any.
	GetHandler(operation string) any
}

// ComponentMatches returns true if the given component string matches the glob pattern.
// The pattern is a single path component (no slashes) and can contain:
//   - "*" matches any sequence of characters within the component
//   - "?" matches any single character within the component
// The matching is performed over the entire component (anchored).
// This is used by the FUSE directory tree for glob pattern matching.
func ComponentMatches(pattern, component string) bool {
	// If pattern contains no wildcards, perform exact match
	if !strings.ContainsAny(pattern, "*?") {
		return pattern == component
	}

	// Convert glob pattern to regex
	regexStr := "^" + globToRegex(pattern) + "$"
	re, err := regexp.Compile(regexStr)
	if err != nil {
		// Should never happen for valid glob patterns
		return false
	}
	return re.MatchString(component)
}

// globToRegex converts a glob pattern with * and ? to a regex pattern.
// It escapes regex special characters except * and ?.
func globToRegex(pattern string) string {
	var result strings.Builder
	for _, ch := range pattern {
		switch ch {
		case '*':
			result.WriteString(".*")
		case '?':
			result.WriteString(".")
		case '.', '+', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			result.WriteString("\\" + string(ch))
		default:
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// Node represents a node in the hierarchical route tree.
// Each node corresponds to a path component (directory or file).
type Node struct {
	// Name is the component name (e.g., "api", "users", "{id}")
	Name string
	// Type indicates whether this node is a directory, file, or parameter.
	Type NodeType
	// Pattern is the full route pattern for leaf nodes (empty for non-leaf nodes).
	Pattern string
	// Children maps child component names to nodes.
	Children map[string]*Node
	// Handlers maps operation names to route handlers.
	// Only leaf nodes (files) have handlers.
	Handlers map[string]any
}

// NewNode creates a new node with the given name and type.
func NewNode(name string, typ NodeType) *Node {
	return &Node{
		Name:     name,
		Type:     typ,
		Pattern:  "",
		Children: make(map[string]*Node),
		Handlers: make(map[string]any),
	}
}

// BuildTree constructs a hierarchical tree from route handlers.
// handlers is a map from operation to map of path pattern to handler.
func BuildTree(handlers map[string]map[string]any) *Node {

	root := NewNode("", NodeTypeDir)

	for op, opRoutes := range handlers {
		for pathPattern, handler := range opRoutes {
			addRouteToTree(root, op, pathPattern, handler)
		}
	}

	return root
}

// addRouteToTree adds a route pattern to the tree.
func addRouteToTree(root *Node, operation, pathPattern string, handler any) {
	parts := splitPath(pathPattern)
	node := root

	for i, part := range parts {
		isLast := i == len(parts)-1
		isParam := strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}")
		isWildcard := part == "*"
		isGlob := (strings.Contains(part, "*") || strings.Contains(part, "?")) && !isWildcard

		var childType NodeType
		if isLast {
			// Leaf component is a file (even if it's a parameter or wildcard)
			childType = NodeTypeFile
		} else {
			// Intermediate component is a directory
			childType = NodeTypeDir
		}
		if isParam {
			// Parameter nodes are special directories/files
			childType = NodeTypeParam
		}
		if isWildcard {
			// Wildcard nodes match any remaining path components
			childType = NodeTypeWildcard
		}
		if isGlob {
			// Glob pattern nodes match components using wildcard patterns
			childType = NodeTypeGlob
		}

		// Ensure child exists
		child, ok := node.Children[part]
		if !ok {
			child = NewNode(part, childType)
			node.Children[part] = child
		} else if !isLast && child.Type == NodeTypeFile {
			// Upgrade file to directory when sub-routes are registered under it.
			child.Type = NodeTypeDir
		}
		node = child
	}

	// At leaf node, store pattern and add handler for this operation
	node.Pattern = pathPattern
	node.Handlers[operation] = handler
}

// splitPath splits a route pattern into components, ignoring empty components.
func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "/")
}

// FindNode finds a node in the tree matching the given path components.
// It follows exact matches first, then parameter nodes.
// Returns the node and a map of parameter values extracted from the path.
func (n *Node) FindNode(components []string) (TreeNode, map[string]string) {
	return n.findNodeRecursive(components, make(map[string]string))
}

func (n *Node) findNodeRecursive(components []string, params map[string]string) (TreeNode, map[string]string) {
	if len(components) == 0 {
		return n, params
	}

	first := components[0]
	rest := components[1:]

	// Try exact match first
	if child, ok := n.Children[first]; ok {
		return child.findNodeRecursive(rest, params)
	}

	// Try glob pattern nodes
	for name, child := range n.Children {
		if child.Type == NodeTypeGlob {
			if ComponentMatches(name, first) {
				// Glob pattern matches, but doesn't capture parameter
				return child.findNodeRecursive(rest, params)
			}
		}
	}

	// Try parameter nodes
	for name, child := range n.Children {
		if child.Type == NodeTypeParam {
			// Extract parameter name (without braces)
			paramName := strings.Trim(name, "{}")
			params[paramName] = first
			return child.findNodeRecursive(rest, params)
		}
	}

	// Try wildcard nodes
	for _, child := range n.Children {
		if child.Type == NodeTypeWildcard {
			// Check if wildcard node is leaf (has Pattern) or intermediate
			if child.Pattern != "" && len(child.Children) == 0 {
				// Leaf wildcard: capture remaining components
				params["*"] = strings.Join(components, "/")
				return child.findNodeRecursive([]string{}, params)
			} else {
				// Intermediate wildcard: match exactly one component
				if len(components) == 0 {
					return nil, nil
				}
				params["*"] = components[0]
				return child.findNodeRecursive(components[1:], params)
			}
		}
	}

	// Not found
	return nil, nil
}

// ListChildren returns the names of immediate children of this node.
func (n *Node) ListChildren() []string {
	var names []string
	for name := range n.Children {
		names = append(names, name)
	}
	return names
}

// GetHandler returns the handler for a given operation, if any.
func (n *Node) GetHandler(operation string) any {
	return n.Handlers[operation]
}

// GetName returns the component name of this node.
func (n *Node) GetName() string {
	return n.Name
}

// GetType returns the type of this node.
func (n *Node) GetType() NodeType {
	return n.Type
}

// GetPattern returns the full route pattern for leaf nodes.
func (n *Node) GetPattern() string {
	return n.Pattern
}

// GetChildren returns the child nodes of this node as TreeNode interface.
func (n *Node) GetChildren() map[string]TreeNode {
	// Convert map[string]*Node to map[string]TreeNode
	// Since *Node implements TreeNode, we can cast each value
	children := make(map[string]TreeNode, len(n.Children))
	for k, v := range n.Children {
		children[k] = v
	}
	return children
}

// GetHandlers returns the operation handlers for this node.
func (n *Node) GetHandlers() map[string]any {
	return n.Handlers
}