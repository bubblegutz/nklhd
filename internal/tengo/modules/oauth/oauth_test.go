package oauth

import (
	"testing"

	"github.com/d5/tengo/v2"
)

func TestOauthClientMissingArgs(t *testing.T) {
	fn := Module["client"].(*tengo.UserFunction)
	_, err := fn.Value()
	if err == nil {
		t.Error("expected error for missing args")
	}
}

func TestOauthClientWithConfig(t *testing.T) {
	fn := Module["client"].(*tengo.UserFunction)
	config := map[string]tengo.Object{
		"client_id": &tengo.String{Value: "test-client"},
		"token_url": &tengo.String{Value: "https://provider.example.com/token"},
		"device_url": &tengo.String{Value: "https://provider.example.com/device"},
		"timeout_ms": &tengo.Int{Value: 5000},
	}
	obj, err := fn.Value(&tengo.ImmutableMap{Value: config})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := obj.(*tengo.ImmutableMap)
	if !ok {
		t.Fatalf("expected map, got %T", obj)
	}
	for _, name := range []string{"device_flow_start", "device_poll", "auth_code_url", "exchange_code"} {
		if _, ok := m.Value[name]; !ok {
			t.Errorf("expected client method %q", name)
		}
	}
}

func TestOauthClientFromMutableMap(t *testing.T) {
	fn := Module["client"].(*tengo.UserFunction)
	config := map[string]tengo.Object{
		"client_id": &tengo.String{Value: "test-client-2"},
		"token_url": &tengo.String{Value: "https://provider.example.com/token"},
	}
	obj, err := fn.Value(&tengo.Map{Value: config})
	if err != nil {
		t.Fatal(err)
	}
	_, ok := obj.(*tengo.ImmutableMap)
	if !ok {
		t.Fatalf("expected map, got %T", obj)
	}
}

func TestOauthClientDefaultTimeout(t *testing.T) {
	fn := Module["client"].(*tengo.UserFunction)
	config := map[string]tengo.Object{
		"client_id": &tengo.String{Value: "test-client-3"},
	}
	obj, err := fn.Value(&tengo.ImmutableMap{Value: config})
	if err != nil {
		t.Fatal(err)
	}
	_, ok := obj.(*tengo.ImmutableMap)
	if !ok {
		t.Fatalf("expected map, got %T", obj)
	}
}
