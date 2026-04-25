package jq

import (
	"testing"

	"github.com/d5/tengo/v2"
)

func makeArray(items []map[string]interface{}) *tengo.Array {
	objs := make([]tengo.Object, len(items))
	for i, item := range items {
		obj, err := tengo.FromInterface(item)
		if err != nil {
			panic(err)
		}
		objs[i] = obj
	}
	return &tengo.Array{Value: objs}
}

func TestJqPluck(t *testing.T) {
	fn := Module["pluck"].(*tengo.UserFunction)
	items := makeArray([]map[string]interface{}{
		{"name": "Alice", "age": 30},
		{"name": "Bob", "age": 25},
	})
	obj, err := fn.Value(items, &tengo.String{Value: "name"})
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("pluck error: %s", errObj.Value.String())
	}
	arr, ok := obj.(*tengo.Array)
	if !ok {
		t.Fatalf("expected array, got %T", obj)
	}
	if len(arr.Value) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr.Value))
	}
}

func TestJqWhere(t *testing.T) {
	fn := Module["where"].(*tengo.UserFunction)
	items := makeArray([]map[string]interface{}{
		{"name": "Alice", "age": 30, "role": "admin"},
		{"name": "Bob", "age": 25, "role": "user"},
		{"name": "Charlie", "age": 35, "role": "admin"},
	})
	obj, err := fn.Value(items, &tengo.String{Value: "role"}, &tengo.String{Value: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("where error: %s", errObj.Value.String())
	}
	arr, ok := obj.(*tengo.Array)
	if !ok {
		t.Fatalf("expected array, got %T", obj)
	}
	if len(arr.Value) != 2 {
		t.Errorf("expected 2 admin items, got %d", len(arr.Value))
	}
}

func TestJqFind(t *testing.T) {
	fn := Module["find"].(*tengo.UserFunction)
	items := makeArray([]map[string]interface{}{
		{"id": 1, "name": "Alice", "age": 30},
		{"id": 2, "name": "Bob", "age": 25},
	})
	obj, err := fn.Value(items, &tengo.String{Value: "name"})
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("find error: %s", errObj.Value.String())
	}
	arr, ok := obj.(*tengo.Array)
	if !ok {
		t.Fatalf("expected array, got %T", obj)
	}
	if len(arr.Value) != 2 {
		t.Errorf("expected 2 find results, got %d", len(arr.Value))
	}
}

func TestJqCount(t *testing.T) {
	fn := Module["count"].(*tengo.UserFunction)
	items := makeArray([]map[string]interface{}{
		{"name": "Alice"},
		{"name": "Bob"},
		{"name": "Charlie"},
	})
	obj, err := fn.Value(items)
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("count error: %s", errObj.Value.String())
	}
	i, ok := obj.(*tengo.Int)
	if !ok {
		t.Fatalf("expected int, got %T", obj)
	}
	if i.Value != 3 {
		t.Errorf("expected count=3, got %d", i.Value)
	}
}

func TestJqHas(t *testing.T) {
	fn := Module["has"].(*tengo.UserFunction)
	items := makeArray([]map[string]interface{}{
		{"name": "Alice", "email": "alice@test.com"},
		{"name": "Bob"},
		{"name": "Charlie", "email": "charlie@test.com"},
	})
	obj, err := fn.Value(items, &tengo.String{Value: "email"})
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("has error: %s", errObj.Value.String())
	}
	arr, ok := obj.(*tengo.Array)
	if !ok {
		t.Fatalf("expected array, got %T", obj)
	}
	if len(arr.Value) != 2 {
		t.Errorf("expected 2 items with email, got %d", len(arr.Value))
	}
}

func TestJqFirstLast(t *testing.T) {
	items := makeArray([]map[string]interface{}{
		{"id": 1, "name": "Alice"},
		{"id": 2, "name": "Bob"},
		{"id": 3, "name": "Charlie"},
	})

	firstFn := Module["first"].(*tengo.UserFunction)
	obj, err := firstFn.Value(items)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := obj.(*tengo.Map)
	if !ok {
		t.Fatalf("expected map for first, got %T", obj)
	}
	if id, ok := m.Value["id"].(*tengo.Int); !ok || id.Value != 1 {
		t.Errorf("expected first id=1, got %v", m.Value["id"])
	}

	lastFn := Module["last"].(*tengo.UserFunction)
	obj, err = lastFn.Value(items)
	if err != nil {
		t.Fatal(err)
	}
	m, ok = obj.(*tengo.Map)
	if !ok {
		t.Fatalf("expected map for last, got %T", obj)
	}
	if id, ok := m.Value["id"].(*tengo.Int); !ok || id.Value != 3 {
		t.Errorf("expected last id=3, got %v", m.Value["id"])
	}
}

func TestJqMissingArgs(t *testing.T) {
	tests := []string{"pluck", "where", "has", "find", "first", "last", "count"}
	for _, name := range tests {
		fn, ok := Module[name].(*tengo.UserFunction)
		if !ok {
			t.Fatalf("%s not a UserFunction", name)
		}
		_, err := fn.Value()
		if err == nil {
			t.Errorf("%s: expected error for no args", name)
		}
	}
}
