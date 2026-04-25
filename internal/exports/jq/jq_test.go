package jq

import (
	"testing"
)

func TestNewQueryAndItems(t *testing.T) {
	items := []interface{}{1, "two", 3.0}
	q := NewQuery(items)
	if len(q.Items()) != 3 {
		t.Errorf("Expected 3 items, got %d", len(q.Items()))
	}
	for i, v := range q.Items() {
		if v != items[i] {
			t.Errorf("Item %d mismatch: expected %v, got %v", i, items[i], v)
		}
	}
}

func TestFindExactKey(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"name": "Alice", "age": 30},
		map[string]interface{}{"name": "Bob", "age": 25},
		map[string]interface{}{"title": "Manager"},
	}
	q := NewQuery(data)
	result := q.Find("name")
	if result.Count() != 2 {
		t.Errorf("Expected 2 matches for 'name', got %d", result.Count())
	}
	// Check values
	items := result.Items()
	expected := []interface{}{"Alice", "Bob"}
	for i, v := range items {
		if v != expected[i] {
			t.Errorf("Item %d mismatch: expected %v, got %v", i, expected[i], v)
		}
	}
}

func TestFindPattern(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"go_version": "1.21", "lang": "Go"},
		map[string]interface{}{"python_version": "3.11", "lang": "Python"},
		map[string]interface{}{"go_module": "fmt"},
	}
	q := NewQuery(data)
	result := q.Find("/go/")
	if result.Count() != 2 {
		t.Errorf("Expected 2 matches for pattern '/go/', got %d", result.Count())
	}
	// Values should be "1.21" and "fmt"
	items := result.Items()
	// Order is map iteration order, not guaranteed. We'll just check presence.
	found := make(map[interface{}]bool)
	for _, v := range items {
		found[v] = true
	}
	if !found["1.21"] || !found["fmt"] {
		t.Errorf("Missing expected values, got %v", items)
	}
}

func TestPluck(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"id": 1, "name": "Alice"},
		map[string]interface{}{"id": 2, "name": "Bob"},
		"not a map",
		map[string]interface{}{"id": 3},
	}
	q := NewQuery(data)
	result := q.Pluck("id")
	items := result.Items()
	expected := []interface{}{1, 2, nil, 3}
	if len(items) != len(expected) {
		t.Fatalf("Expected %d items, got %d", len(expected), len(items))
	}
	for i, v := range items {
		if v != expected[i] {
			t.Errorf("Item %d mismatch: expected %v, got %v", i, expected[i], v)
		}
	}
}

func TestWhere(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"role": "admin", "name": "Alice"},
		map[string]interface{}{"role": "user", "name": "Bob"},
		map[string]interface{}{"role": "admin", "name": "Charlie"},
		"not a map",
	}
	q := NewQuery(data)
	result := q.Where("role", "admin")
	if result.Count() != 2 {
		t.Errorf("Expected 2 admin matches, got %d", result.Count())
	}
	items := result.Items()
	for _, v := range items {
		m := v.(map[string]interface{})
		if m["role"] != "admin" {
			t.Errorf("Expected role admin, got %v", m["role"])
		}
	}
}

func TestWhereNumeric(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"score": 100},
		map[string]interface{}{"score": 200},
		map[string]interface{}{"score": 100.0}, // float64
		map[string]interface{}{"score": 150},
	}
	q := NewQuery(data)
	result := q.Where("score", 100)
	if result.Count() != 2 {
		t.Errorf("Expected 2 matches for score 100 (int and float), got %d", result.Count())
	}
}

func TestHas(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{"name": "Alice", "age": 30},
		map[string]interface{}{"name": "Bob"},
		map[string]interface{}{"age": 25},
	}
	q := NewQuery(data)
	result := q.Has("age")
	if result.Count() != 2 {
		t.Errorf("Expected 2 items with age key, got %d", result.Count())
	}
}

func TestFirstLast(t *testing.T) {
	data := []interface{}{"a", "b", "c"}
	q := NewQuery(data)
	if q.First() != "a" {
		t.Errorf("First() expected 'a', got %v", q.First())
	}
	if q.Last() != "c" {
		t.Errorf("Last() expected 'c', got %v", q.Last())
	}
	empty := NewQuery(nil)
	if empty.First() != nil {
		t.Errorf("First() on empty query expected nil, got %v", empty.First())
	}
	if empty.Last() != nil {
		t.Errorf("Last() on empty query expected nil, got %v", empty.Last())
	}
}

func TestCount(t *testing.T) {
	q := NewQuery([]interface{}{1, 2, 3, 4})
	if q.Count() != 4 {
		t.Errorf("Count() expected 4, got %d", q.Count())
	}
	empty := NewQuery(nil)
	if empty.Count() != 0 {
		t.Errorf("Count() on empty query expected 0, got %d", empty.Count())
	}
}