// Package jq provides jQuery-like query operations for generic Go data (maps and slices).
package jq

import (
	"strings"
)

// Query represents a collection of Go values that can be filtered and transformed.
type Query struct {
	items []interface{}
}

// NewQuery creates a Query from a slice of Go values.
func NewQuery(items []interface{}) *Query {
	return &Query{items: items}
}

// Items returns the underlying Go values.
func (q *Query) Items() []interface{} {
	return q.items
}

// isMap returns true if v is a map with string keys.
func isMap(v interface{}) bool {
	_, ok := v.(map[string]interface{})
	return ok
}

// isSlice returns true if v is a slice.
func isSlice(v interface{}) bool {
	_, ok := v.([]interface{})
	return ok
}

// getMapField returns the value associated with key in map m, or nil if not present or m is not a map.
func getMapField(m interface{}, key string) interface{} {
	if m, ok := m.(map[string]interface{}); ok {
		return m[key]
	}
	return nil
}

// valueEquals compares two arbitrary Go values for equality.
func valueEquals(a, b interface{}) bool {
	if a == nil || b == nil {
		return a == b
	}
	switch av := a.(type) {
	case bool:
		if bv, ok := b.(bool); ok {
			return av == bv
		}
	case float64:
		if bv, ok := b.(float64); ok {
			return av == bv
		}
		// Also compare with int if b is int (converted from JSON)
		if bv, ok := b.(int); ok {
			return av == float64(bv)
		}
		if bv, ok := b.(int64); ok {
			return av == float64(bv)
		}
	case int:
		if bv, ok := b.(int); ok {
			return av == bv
		}
		if bv, ok := b.(float64); ok {
			return float64(av) == bv
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return av == bv
		}
		if bv, ok := b.(float64); ok {
			return float64(av) == bv
		}
	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}
	default:
		// For slices and maps, compare pointer equality (deep equality would be expensive).
		// In practice, jq queries use simple scalar values for equality.
		return false
	}
	return false
}

// Find returns a new Query containing values from nested maps where the key matches the given pattern.
// If pattern starts and ends with '/', it's treated as a substring match.
// Only map[string]interface{} items are examined; other items are skipped.
func (q *Query) Find(key string) *Query {
	isPattern := false
	pat := key
	if len(key) > 1 && key[0] == '/' && key[len(key)-1] == '/' {
		isPattern = true
		pat = key[1 : len(key)-1]
	}
	out := make([]interface{}, 0)
	for _, it := range q.items {
		if !isMap(it) {
			continue
		}
		m := it.(map[string]interface{})
		for k, v := range m {
			if isPattern {
				if strings.Contains(k, pat) {
					out = append(out, v)
				}
			} else {
				if k == key {
					out = append(out, v)
				}
			}
		}
	}
	return NewQuery(out)
}

// Pluck returns a new Query containing the value of the given key from each map item.
// Non‑map items produce nil in the result.
func (q *Query) Pluck(key string) *Query {
	out := make([]interface{}, 0, len(q.items))
	for _, v := range q.items {
		out = append(out, getMapField(v, key))
	}
	return NewQuery(out)
}

// Where returns a new Query containing items where the given key equals the given value.
// Only map items are considered.
func (q *Query) Where(key string, val interface{}) *Query {
	out := make([]interface{}, 0)
	for _, v := range q.items {
		if !isMap(v) {
			continue
		}
		if valueEquals(getMapField(v, key), val) {
			out = append(out, v)
		}
	}
	return NewQuery(out)
}

// Has returns a new Query containing items that have the given key (value is not nil).
// Only map items are considered.
func (q *Query) Has(key string) *Query {
	out := make([]interface{}, 0)
	for _, v := range q.items {
		if getMapField(v, key) != nil {
			out = append(out, v)
		}
	}
	return NewQuery(out)
}

// First returns the first item, or nil if the Query is empty.
func (q *Query) First() interface{} {
	if len(q.items) == 0 {
		return nil
	}
	return q.items[0]
}

// Last returns the last item, or nil if the Query is empty.
func (q *Query) Last() interface{} {
	if len(q.items) == 0 {
		return nil
	}
	return q.items[len(q.items)-1]
}

// Count returns the number of items in the Query.
func (q *Query) Count() int {
	return len(q.items)
}