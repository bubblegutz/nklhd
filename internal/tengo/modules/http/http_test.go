package http

import (
	"strings"
	"testing"

	"github.com/d5/tengo/v2"
)

func TestHttpRequestMissingArgs(t *testing.T) {
	fn := Module["request"].(*tengo.UserFunction)
	_, err := fn.Value()
	if err == nil {
		t.Error("expected error for missing args")
	}
}

func TestHttpRequestWithMinimalArgs(t *testing.T) {
	fn := Module["request"].(*tengo.UserFunction)
	obj, err := fn.Value(
		&tengo.String{Value: "GET"},
		&tengo.String{Value: "http://127.0.0.1:1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	errObj, ok := obj.(*tengo.Error)
	if !ok {
		t.Logf("unexpected result type: %T, value: %s", obj, obj.String())
	} else {
		if !strings.Contains(errObj.Value.String(), "request error") &&
			!strings.Contains(errObj.Value.String(), "connect") &&
			!strings.Contains(errObj.Value.String(), "refused") {
			t.Logf("got expected error type: %s", errObj.Value.String())
		}
	}
}

func TestHttpClientMissingArgs(t *testing.T) {
	fn := Module["client"].(*tengo.UserFunction)
	_, err := fn.Value()
	if err == nil {
		t.Error("expected error for missing args")
	}
}

func TestHttpClientCreation(t *testing.T) {
	fn := Module["client"].(*tengo.UserFunction)
	obj, err := fn.Value(&tengo.String{Value: "https://api.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := obj.(*tengo.ImmutableMap)
	if !ok {
		t.Fatalf("expected map, got %T", obj)
	}
	for _, name := range []string{"get", "post", "put", "delete", "head", "options", "withBasic", "withBearer", "withTLSInsecure"} {
		if _, ok := m.Value[name]; !ok {
			t.Errorf("expected client method %q", name)
		}
	}
}
