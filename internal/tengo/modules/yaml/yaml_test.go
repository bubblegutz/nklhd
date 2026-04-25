package yaml

import (
	"strings"
	"testing"

	"github.com/d5/tengo/v2"
)

func TestYamlEncode(t *testing.T) {
	fn := Module["encode"].(*tengo.UserFunction)
	data, err := tengo.FromInterface(map[string]interface{}{"name": "alice", "age": 30})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := fn.Value(data)
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("encode error: %s", errObj.Value.String())
	}
	s, ok := obj.(*tengo.String)
	if !ok {
		t.Fatalf("expected string, got %T", obj)
	}
	if !strings.Contains(s.Value, "alice") {
		t.Errorf("expected 'alice' in output, got: %s", s.Value)
	}
}

func TestYamlDecode(t *testing.T) {
	fn := Module["decode"].(*tengo.UserFunction)
	obj, err := fn.Value(&tengo.String{Value: "name: bob\nscore: 95.5\n"})
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("decode error: %s", errObj.Value.String())
	}
	m, ok := obj.(*tengo.Map)
	if !ok {
		t.Fatalf("expected map, got %T", obj)
	}
	name, ok := m.Value["name"].(*tengo.String)
	if !ok || name.Value != "bob" {
		t.Errorf("expected name=bob, got %v", m.Value["name"])
	}
}

func TestYamlFromJSON(t *testing.T) {
	fn := Module["fromJSON"].(*tengo.UserFunction)
	obj, err := fn.Value(&tengo.String{Value: `{"name":"bob","score":95.5}`})
	if err != nil {
		t.Fatal(err)
	}
	if errObj, ok := obj.(*tengo.Error); ok {
		t.Fatalf("fromJSON error: %s", errObj.Value.String())
	}
	s, ok := obj.(*tengo.String)
	if !ok {
		t.Fatalf("expected string, got %T", obj)
	}
	if !strings.Contains(s.Value, "bob") {
		t.Errorf("expected 'bob' in output, got: %s", s.Value)
	}
}

func TestYamlDecodeInvalid(t *testing.T) {
	fn := Module["decode"].(*tengo.UserFunction)
	obj, err := fn.Value(&tengo.String{Value: "invalid: [yaml"})
	if err != nil {
		t.Fatal(err)
	}
	errObj, ok := obj.(*tengo.Error)
	if !ok {
		t.Fatalf("expected error for invalid yaml, got %T", obj)
	}
	if !strings.Contains(errObj.Value.String(), "decode error") {
		t.Errorf("expected decode error, got: %s", errObj.Value.String())
	}
}

func TestYamlMissingArgs(t *testing.T) {
	fn := Module["encode"].(*tengo.UserFunction)
	_, err := fn.Value()
	if err == nil {
		t.Error("expected error for missing args")
	}
}
