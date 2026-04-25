// Package router provides a radix tree implementation based on go-chi/chi.
// It's been modified to support generic operation strings instead of HTTP methods.
package router

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Radix tree implementation below is a based on the original work by
// Armon Dadgar in https://github.com/armon/go-radix/blob/master/radix.go
// (MIT licensed). It's been heavily modified for use as a HTTP routing tree.
// Further modified for nklhd to support generic operation strings.

// RouteContext returns the routing Context object from a context.Context.
func RouteContext(ctx context.Context) *Context {
	val, _ := ctx.Value(RouteCtxKey).(*Context)
	return val
}

// NewRouteContext returns a new routing Context object.
func NewRouteContext() *Context {
	return &Context{}
}

var (
	// RouteCtxKey is the context.Context key to store the request context.
	RouteCtxKey = &contextKey{"RouteContext"}
)

// Context is the default routing context set on the root node of a
// request context to track route patterns, URL parameters and
// an optional routing path.
type Context struct {
	Routes Routes

	// parentCtx is the parent of this one, for using Context as a
	// context.Context directly. This is an optimization that saves
	// 1 allocation.
	parentCtx context.Context

	// Routing path override used during the route search.
	RoutePath string

	// URLParams are the stack of routeParams captured during the
	// routing lifecycle across a stack of sub-routers.
	URLParams RouteParams

	// Route parameters matched for the current sub-router. It is
	// intentionally unexported so it can't be tampered.
	routeParams RouteParams

	// The endpoint routing pattern that matched the request URI path
	// or `RoutePath` of the current sub-router. This value will update
	// during the lifecycle of a request passing through a stack of
	// sub-routers.
	routePattern string

	// Routing pattern stack throughout the lifecycle of the request,
	// across all connected routers. It is a record of all matching
	// patterns across a stack of sub-routers.
	RoutePatterns []string
}

// Reset a routing context to its initial state.
func (x *Context) Reset() {
	x.Routes = nil
	x.RoutePath = ""
	x.RoutePatterns = x.RoutePatterns[:0]
	x.URLParams.Keys = x.URLParams.Keys[:0]
	x.URLParams.Values = x.URLParams.Values[:0]

	x.routePattern = ""
	x.routeParams.Keys = x.routeParams.Keys[:0]
	x.routeParams.Values = x.routeParams.Values[:0]
	x.parentCtx = nil
}

// URLParam returns the corresponding URL parameter value from the request
// routing context.
func (x *Context) URLParam(key string) string {
	for k := len(x.URLParams.Keys) - 1; k >= 0; k-- {
		if x.URLParams.Keys[k] == key {
			return x.URLParams.Values[k]
		}
	}
	return ""
}

// RoutePattern builds the routing pattern string for the particular
// request, at the particular point during routing. This means, the value
// will change throughout the execution of a request in a router. That is
// why it's advised to only use this value after calling the next handler.
func (x *Context) RoutePattern() string {
	if x == nil {
		return ""
	}
	routePattern := strings.Join(x.RoutePatterns, "")
	routePattern = replaceWildcards(routePattern)
	if routePattern != "/" {
		routePattern = strings.TrimSuffix(routePattern, "//")
		routePattern = strings.TrimSuffix(routePattern, "/")
	}
	return routePattern
}

// replaceWildcards takes a route pattern and replaces all occurrences of
// "/*/" with "/". It iteratively runs until no wildcards remain to
// correctly handle consecutive wildcards.
func replaceWildcards(p string) string {
	for strings.Contains(p, "/*/") {
		p = strings.ReplaceAll(p, "/*/", "/")
	}
	return p
}

// RouteParams is a structure to track URL routing parameters efficiently.
type RouteParams struct {
	Keys, Values []string
}

// Add will append a URL parameter to the end of the route param
func (s *RouteParams) Add(key, value string) {
	s.Keys = append(s.Keys, key)
	s.Values = append(s.Values, value)
}

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an any without allocation. This technique
// for defining context keys was copied from Go 1.7's new use of context in net/http.
type contextKey struct {
	name string
}

func (k *contextKey) String() string {
	return "router context value " + k.name
}

type nodeTyp uint8

const (
	ntStatic   nodeTyp = iota // /home
	ntRegexp                  // /{id:[0-9]+}
	ntParam                   // /{user}
	ntGlob                    // /file*.txt, /*.txt, etc.
	ntCatchAll                // /api/v1/*
)

type node struct {
	// subroutes on the leaf node
	subroutes Routes

	// regexp matcher for regexp nodes
	rex *regexp.Regexp

	// operation handler endpoints on the leaf node
	endpoints endpoints

	// prefix is the common prefix we ignore
	prefix string

	// child nodes should be stored in-order for iteration,
	// in groups of the node type.
	children [ntCatchAll + 1]nodes

	// first byte of the child prefix
	tail byte

	// node type: static, regexp, param, catchAll
	typ nodeTyp

	// first byte of the prefix
	label byte
}

// endpoints is a mapping of operation strings to handlers for a given route.
type endpoints map[string]*endpoint

type endpoint struct {
	// endpoint handler
	handler any

	// pattern is the routing pattern for handler nodes
	pattern string

	// parameter keys recorded on handler nodes
	paramKeys []string
}

func (s endpoints) Value(op string) *endpoint {
	mh, ok := s[op]
	if !ok {
		mh = &endpoint{}
		s[op] = mh
	}
	return mh
}

func (n *node) InsertRoute(op string, pattern string, handler any) *node {
	var parent *node
	search := pattern

	for {
		// Handle key exhaustion
		if len(search) == 0 {
			// Insert or update the node's leaf handler
			n.setEndpoint(op, handler, pattern)
			return n
		}

		// We're going to be searching for a wild node next,
		// in this case, we need to get the tail
		var label = search[0]
		var segTail byte
		var segEndIdx int
		var segTyp nodeTyp
		var segRexpat string
		if label == '{' || label == '*' {
			segTyp, _, segRexpat, segTail, _, segEndIdx = patNextSegment(search)
		}

		var prefix string
		if segTyp == ntRegexp {
			prefix = segRexpat
		}

		// Look for the edge to attach to
		parent = n
		n = n.getEdge(segTyp, label, segTail, prefix)

		// No edge, create one
		if n == nil {
			child := &node{label: label, tail: segTail, prefix: search}
			hn := parent.addChild(child, search)
			hn.setEndpoint(op, handler, pattern)

			return hn
		}

		// Found an edge to match the pattern

		if n.typ > ntStatic {
			// We found a param node, trim the param from the search path and continue.
			// This param/wild pattern segment would already be on the tree from a previous
			// call to addChild when creating a new node.
			search = search[segEndIdx:]
			continue
		}

		// Static nodes fall below here.
		// Determine longest prefix of the search key on match.
		commonPrefix := longestPrefix(search, n.prefix)
		if commonPrefix == len(n.prefix) {
			// the common prefix is as long as the current node's prefix we're attempting to insert.
			// keep the search going.
			search = search[commonPrefix:]
			continue
		}

		// Split the node
		child := &node{
			typ:    ntStatic,
			prefix: search[:commonPrefix],
		}
		parent.replaceChild(search[0], segTail, child)

		// Restore the existing node
		n.label = n.prefix[commonPrefix]
		n.prefix = n.prefix[commonPrefix:]
		child.addChild(n, n.prefix)

		// If the new key is a subset, set the method/handler on this node and finish.
		search = search[commonPrefix:]
		if len(search) == 0 {
			child.setEndpoint(op, handler, pattern)
			return child
		}

		// Create a new edge for the node
		subchild := &node{
			typ:    ntStatic,
			label:  search[0],
			prefix: search,
		}
		hn := child.addChild(subchild, search)
		hn.setEndpoint(op, handler, pattern)
		return hn
	}
}

// addChild appends the new `child` node to the tree using the `pattern` as the trie key.
// For a URL router like chi's, we split the static, param, regexp and wildcard segments
// into different nodes. In addition, addChild will recursively call itself until every
// pattern segment is added to the url pattern tree as individual nodes, depending on type.
func (n *node) addChild(child *node, prefix string) *node {
	search := prefix

	// handler leaf node added to the tree is the child.
	// this may be overridden later down the flow
	hn := child

	// Parse next segment
	segTyp, _, segRexpat, segTail, segStartIdx, segEndIdx := patNextSegment(search)

	// Add child depending on next up segment
	switch segTyp {

	case ntStatic:
		// Search prefix is all static (that is, has no params in path)
		// noop

	default:
		// Search prefix contains a param, regexp or wildcard

		if segTyp == ntRegexp {
			rex, err := regexp.Compile(segRexpat)
			if err != nil {
				panic(fmt.Sprintf("router: invalid regexp pattern '%s' in route param", segRexpat))
			}
			child.prefix = segRexpat
			child.rex = rex
		}

		if segStartIdx == 0 {
			// Route starts with a param
			child.typ = segTyp

			if segTyp == ntCatchAll {
				segStartIdx = -1
			} else {
				segStartIdx = segEndIdx
			}
			if segStartIdx < 0 {
				segStartIdx = len(search)
			}
			child.tail = segTail // for params, we set the tail

			if segStartIdx != len(search) {
				// add static edge for the remaining part, split the end.
				// its not possible to have adjacent param nodes, so its certainly
				// going to be a static node next.

				search = search[segStartIdx:] // advance search position

				nn := &node{
					typ:    ntStatic,
					label:  search[0],
					prefix: search,
				}
				hn = child.addChild(nn, search)
			}

		} else if segStartIdx > 0 {
			// Route has some param

			// starts with a static segment
			child.typ = ntStatic
			child.prefix = search[:segStartIdx]
			child.rex = nil

			// add the param edge node
			search = search[segStartIdx:]

			nn := &node{
				typ:   segTyp,
				label: search[0],
				tail:  segTail,
			}
			hn = child.addChild(nn, search)

		}
	}

	n.children[child.typ] = append(n.children[child.typ], child)
	n.children[child.typ].Sort()
	return hn
}

func (n *node) replaceChild(label, tail byte, child *node) {
	for i := 0; i < len(n.children[child.typ]); i++ {
		if n.children[child.typ][i].label == label && n.children[child.typ][i].tail == tail {
			n.children[child.typ][i] = child
			n.children[child.typ][i].label = label
			n.children[child.typ][i].tail = tail
			return
		}
	}
	panic("router: replacing missing child")
}

func (n *node) getEdge(ntyp nodeTyp, label, tail byte, prefix string) *node {
	nds := n.children[ntyp]
	for i := range nds {
		if nds[i].label == label && nds[i].tail == tail {
			if ntyp == ntRegexp && nds[i].prefix != prefix {
				continue
			}
			return nds[i]
		}
	}
	return nil
}

func (n *node) setEndpoint(op string, handler any, pattern string) {
	// Set the handler for the operation type on the node
	if n.endpoints == nil {
		n.endpoints = make(endpoints)
	}

	paramKeys := patParamKeys(pattern)

	h := n.endpoints.Value(op)
	h.handler = handler
	h.pattern = pattern
	h.paramKeys = paramKeys
}

func (n *node) FindRoute(rctx *Context, op string, path string) (*node, endpoints, any) {
	// Reset the context routing pattern and params
	rctx.routePattern = ""
	rctx.routeParams.Keys = rctx.routeParams.Keys[:0]
	rctx.routeParams.Values = rctx.routeParams.Values[:0]

	// Find the routing handlers for the path
	rn := n.findRoute(rctx, op, path)
	if rn == nil {
		return nil, nil, nil
	}

	// Record the routing params in the request lifecycle
	rctx.URLParams.Keys = append(rctx.URLParams.Keys, rctx.routeParams.Keys...)
	rctx.URLParams.Values = append(rctx.URLParams.Values, rctx.routeParams.Values...)

	// Record the routing pattern in the request lifecycle
	if rn.endpoints[op] != nil && rn.endpoints[op].pattern != "" {
		rctx.routePattern = rn.endpoints[op].pattern
		rctx.RoutePatterns = append(rctx.RoutePatterns, rctx.routePattern)
	}

	return rn, rn.endpoints, rn.endpoints[op].handler
}

// Recursive edge traversal by checking all nodeTyp groups along the way.
// It's like searching through a multi-dimensional radix trie.
func (n *node) findRoute(rctx *Context, op string, path string) *node {
	nn := n
	search := path

	for t, nds := range nn.children {
		ntyp := nodeTyp(t)
		if len(nds) == 0 {
			continue
		}

		var xn *node
		xsearch := search

		var label byte
		if search != "" {
			label = search[0]
		}

		switch ntyp {
		case ntStatic:
			xn = nds.findEdge(label)
			if xn == nil || !strings.HasPrefix(xsearch, xn.prefix) {
				continue
			}
			xsearch = xsearch[len(xn.prefix):]

		case ntParam, ntRegexp:
			// short-circuit and return no matching route for empty param values
			if xsearch == "" {
				continue
			}

			// serially loop through each node grouped by the tail delimiter
			for _, xn = range nds {
				// label for param nodes is the delimiter byte
				p := strings.IndexByte(xsearch, xn.tail)

				if p < 0 {
					if xn.tail == '/' {
						p = len(xsearch)
					} else {
						continue
					}
				} else if ntyp == ntRegexp && p == 0 {
					continue
				}

				if ntyp == ntRegexp && xn.rex != nil {
					if !xn.rex.MatchString(xsearch[:p]) {
						continue
					}
				} else if strings.IndexByte(xsearch[:p], '/') != -1 {
					// avoid a match across path segments
					continue
				}

				prevlen := len(rctx.routeParams.Values)
				rctx.routeParams.Values = append(rctx.routeParams.Values, xsearch[:p])
				xsearch = xsearch[p:]

				if len(xsearch) == 0 {
					if xn.isLeaf() {
						h := xn.endpoints[op]
						if h != nil && h.handler != nil {
							rctx.routeParams.Keys = append(rctx.routeParams.Keys, h.paramKeys...)
							return xn
						}
					}
				}

				// recursively find the next node on this branch
				fin := xn.findRoute(rctx, op, xsearch)
				if fin != nil {
					return fin
				}

				// not found on this branch, reset vars
				rctx.routeParams.Values = rctx.routeParams.Values[:prevlen]
				xsearch = search
			}

			rctx.routeParams.Values = append(rctx.routeParams.Values, "")

		default:
			// catch-all nodes
			rctx.routeParams.Values = append(rctx.routeParams.Values, search)
			xn = nds[0]
			xsearch = ""
		}

		if xn == nil {
			continue
		}

		// did we find it yet?
		if len(xsearch) == 0 {
			if xn.isLeaf() {
				h := xn.endpoints[op]
				if h != nil && h.handler != nil {
					rctx.routeParams.Keys = append(rctx.routeParams.Keys, h.paramKeys...)
					return xn
				}
			}
		}

		// recursively find the next node..
		fin := xn.findRoute(rctx, op, xsearch)
		if fin != nil {
			return fin
		}

		// Did not find final handler, let's remove the param here if it was set
		if xn.typ > ntStatic {
			if len(rctx.routeParams.Values) > 0 {
				rctx.routeParams.Values = rctx.routeParams.Values[:len(rctx.routeParams.Values)-1]
			}
		}

	}
	return nil
}

func (n *node) isLeaf() bool {
	return n.endpoints != nil
}

func (n *node) routes() []Route {
	rts := []Route{}

	n.walk(func(eps endpoints, subroutes Routes) bool {
		if eps == nil && subroutes == nil {
			return false
		}
		// Group handlers by unique patterns
		pats := make(map[string]endpoints)

		for op, h := range eps {
			if h.pattern == "" {
				continue
			}
			p, ok := pats[h.pattern]
			if !ok {
				p = endpoints{}
				pats[h.pattern] = p
			}
			p[op] = h
		}


		for p, mh := range pats {
			hs := make(map[string]any)
			for op, h := range mh {
				if h.handler == nil {
					continue
				}
				hs[op] = h.handler
			}

			rt := Route{subroutes, hs, p}
			rts = append(rts, rt)
		}

		return false
	})

	return rts
}

func (n *node) walk(fn func(eps endpoints, subroutes Routes) bool) bool {
	// Visit the leaf values if any
	if (n.endpoints != nil || n.subroutes != nil) && fn(n.endpoints, n.subroutes) {
		return true
	}

	// Recurse on the children
	for _, ns := range n.children {
		for _, cn := range ns {
			if cn.walk(fn) {
				return true
			}
		}
	}
	return false
}

// patNextSegment returns the next segment details from a pattern:
// node type, param key, regexp string, param tail byte, param starting index, param ending index
func patNextSegment(pattern string) (nodeTyp, string, string, byte, int, int) {
	ps := strings.Index(pattern, "{")
	ws := strings.Index(pattern, "*")

	if ps < 0 && ws < 0 {
		return ntStatic, "", "", 0, 0, len(pattern) // we return the entire thing
	}

	// Sanity check
	if ps >= 0 && ws >= 0 && ws < ps {
		panic("router: wildcard '*' must be the last pattern in a route, otherwise use a '{param}'")
	}

	var tail byte = '/' // Default endpoint tail to / byte

	if ps >= 0 {
		// Param/Regexp pattern is next
		nt := ntParam

		// Read to closing } taking into account opens and closes in curl count (cc)
		cc := 0
		pe := ps
		for i, c := range pattern[ps:] {
			if c == '{' {
				cc++
			} else if c == '}' {
				cc--
				if cc == 0 {
					pe = ps + i
					break
				}
			}
		}
		if pe == ps {
			panic("router: route param closing delimiter '}' is missing")
		}

		key := pattern[ps+1 : pe]
		pe++ // set end to next position

		if pe < len(pattern) {
			tail = pattern[pe]
		}

		key, rexpat, isRegexp := strings.Cut(key, ":")
		if isRegexp {
			nt = ntRegexp
		}

		if len(rexpat) > 0 {
			if rexpat[0] != '^' {
				rexpat = "^" + rexpat
			}
			if rexpat[len(rexpat)-1] != '$' {
				rexpat += "$"
			}
		}

		return nt, key, rexpat, tail, ps, pe
	}

	// Wildcard pattern as finale
	if ws < len(pattern)-1 {
		panic("router: wildcard '*' must be the last value in a route. trim trailing text or use a '{param}' instead")
	}
	return ntCatchAll, "*", "", 0, ws, len(pattern)
}

func patParamKeys(pattern string) []string {
	pat := pattern
	paramKeys := []string{}
	for {
		ptyp, paramKey, _, _, _, e := patNextSegment(pat)
		if ptyp == ntStatic {
			return paramKeys
		}
		for i := 0; i < len(paramKeys); i++ {
			if paramKeys[i] == paramKey {
				panic(fmt.Sprintf("router: routing pattern '%s' contains duplicate param key, '%s'", pattern, paramKey))
			}
		}
		paramKeys = append(paramKeys, paramKey)
		pat = pat[e:]
	}
}

// longestPrefix finds the length of the shared prefix of two strings
func longestPrefix(k1, k2 string) (i int) {
	n := len(k1)
	if len(k2) < n {
		n = len(k2)
	}
	for i = 0; i < n; i++ {
		if k1[i] != k2[i] {
			break
		}
	}
	return
}

type nodes []*node

// Sort the list of nodes by label
func (ns nodes) Sort()              { sort.Sort(ns); ns.tailSort() }
func (ns nodes) Len() int           { return len(ns) }
func (ns nodes) Swap(i, j int)      { ns[i], ns[j] = ns[j], ns[i] }
func (ns nodes) Less(i, j int) bool { return ns[i].label < ns[j].label }

// tailSort pushes nodes with '/' as the tail to the end of the list for param nodes.
// The list order determines the traversal order.
func (ns nodes) tailSort() {
	for i := len(ns) - 1; i >= 0; i-- {
		if ns[i].typ > ntStatic && ns[i].tail == '/' {
			ns.Swap(i, len(ns)-1)
			return
		}
	}
}

func (ns nodes) findEdge(label byte) *node {
	num := len(ns)
	idx := 0
	i, j := 0, num-1
	for i <= j {
		idx = i + (j-i)/2
		if label > ns[idx].label {
			i = idx + 1
		} else if label < ns[idx].label {
			j = idx - 1
		} else {
			i = num // breaks cond
		}
	}
	if ns[idx].label != label {
		return nil
	}
	return ns[idx]
}

// Route describes the details of a routing handler.
// Handlers map key is an operation string
type Route struct {
	SubRoutes Routes
	Handlers  map[string]any
	Pattern   string
}

// Routes is an interface for traversing the routing tree.
type Routes interface {
	// Routes returns the routing tree in an easily traversable structure.
	Routes() []Route
}

// convertGlobSegment converts a glob pattern segment to a regex pattern.
// Glob patterns can contain:
//   - "*" matches any sequence of characters within the segment
//   - "?" matches any single character within the segment
// The segment is a path component (does not contain '/').
func convertGlobSegment(segment string) (string, bool) {
	if !strings.ContainsAny(segment, "*?") {
		return segment, false
	}
	
	// Escape regex special characters except * and ?
	// We'll build the regex piece by piece.
	var result strings.Builder
	result.WriteString("^") // Start of segment
	
	for i := 0; i < len(segment); i++ {
		ch := segment[i]
		switch ch {
		case '*':
			result.WriteString(".*")
		case '?':
			result.WriteString(".")
		case '.', '+', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			result.WriteByte('\\')
			result.WriteByte(ch)
		default:
			result.WriteByte(ch)
		}
	}
	
	result.WriteString("$") // End of segment
	return result.String(), true
}

// ConvertGlobPattern converts a path pattern with glob segments to a chi pattern.
// Chi patterns support {param} placeholders, {param:regex}, and * catch-all.
// Glob segments are converted to regex parameters with generated param names.
// Standalone "*" components are handled as follows:
//   - If it's the last component → keep as "*" (catch‑all)
//   - Otherwise → convert to {_wildcardN:.*} (matches exactly one segment)
// Returns the converted pattern and a map from generated param names to original glob segments.
func ConvertGlobPattern(pattern string) (string, map[string]string) {
	if !strings.ContainsAny(pattern, "*?") {
		return pattern, nil
	}
	
	parts := strings.Split(pattern, "/")
	convertedParts := make([]string, 0, len(parts))
	paramMap := make(map[string]string)
	wildcardIndex := 0
	globIndex := 0
	
	for idx, part := range parts {
		if part == "" {
			convertedParts = append(convertedParts, "")
			continue
		}
		// Standalone "*" component
		if part == "*" {
			if idx == len(parts)-1 {
				// Last component → keep as catch‑all
				convertedParts = append(convertedParts, "*")
			} else {
				// Intermediate wildcard → convert to param
				paramName := fmt.Sprintf("_wildcard%d", wildcardIndex)
				wildcardIndex++
				convertedParts = append(convertedParts, "{"+paramName+":.*}")
				paramMap[paramName] = part
			}
			continue
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			// Already a parameter - keep as is
			convertedParts = append(convertedParts, part)
			continue
		}
		
		// Check if segment contains glob characters
		if regex, isGlob := convertGlobSegment(part); isGlob {
			paramName := fmt.Sprintf("_glob%d", globIndex)
			globIndex++
			convertedParts = append(convertedParts, "{"+paramName+":"+regex+"}")
			paramMap[paramName] = part
		} else {
			convertedParts = append(convertedParts, part)
		}
	}
	
	return strings.Join(convertedParts, "/"), paramMap
}

// MustCompileGlobPattern converts a glob pattern and validates the resulting regex.
func MustCompileGlobPattern(pattern string) string {
	converted, _ := ConvertGlobPattern(pattern)
	// Validate regex by trying to compile (chi will do this anyway)
	if strings.Contains(converted, "{") {
		// Extract regex patterns and compile them
		// Simplified: just try to compile the whole pattern as a route
		// by inserting into a dummy tree node
		dummy := &node{}
		// This will panic if regex is invalid
		dummy.InsertRoute("dummy", converted, nil)
	}
	return converted
}

