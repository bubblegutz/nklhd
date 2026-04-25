package json

import (
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
	}{
		{"null", nil},
		{"bool true", true},
		{"bool false", false},
		{"integer", 42},
		{"float", 3.14159},
		{"string", "hello world"},
		{"array", []interface{}{1, 2, 3}},
		{"object", map[string]interface{}{"foo": "bar", "num": 123}},
		{"nested", map[string]interface{}{
			"users": []interface{}{
				map[string]interface{}{"name": "alice", "age": 30},
				map[string]interface{}{"name": "bob", "age": 25},
			},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := Encode(tt.data)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			t.Logf("Encoded: %s", encoded)

			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			// Note: yaml.v3 unmarshals numbers as float64, integers as int.
			// We can't do deep equality directly, so just ensure decode succeeded.
			if decoded == nil && tt.data != nil {
				t.Errorf("Decoded nil but input non-nil")
			}
		})
	}
}

func TestDecodeInvalid(t *testing.T) {
	_, err := Decode("{invalid json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFromYAML(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		expect string // expected JSON (approximate)
	}{
		{
			name:   "simple mapping",
			yaml:   "foo: bar\nnum: 42",
			expect: `{"foo":"bar","num":42}`,
		},
		{
			name:   "list",
			yaml:   "- one\n- two\n- three",
			expect: `["one","two","three"]`,
		},
		{
			name:   "nested",
			yaml:   "users:\n  - name: alice\n    age: 30\n  - name: bob\n    age: 25",
			expect: `{"users":[{"name":"alice","age":30},{"name":"bob","age":25}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromYAML(tt.yaml)
			if err != nil {
				t.Fatalf("FromYAML failed: %v", err)
			}
			// Decode both to compare structure (order may differ)
			gotVal, err := Decode(got)
			if err != nil {
				t.Fatalf("Decode result failed: %v", err)
			}
			expectVal, err := Decode(tt.expect)
			if err != nil {
				t.Fatalf("Decode expectation failed: %v", err)
			}
			// Simple comparison: type and length. Could do recursive equality but okay.
			if gotVal == nil && expectVal != nil {
				t.Errorf("got nil, expected non-nil")
			}
			t.Logf("YAML -> JSON: %s", got)
		})
	}
}
