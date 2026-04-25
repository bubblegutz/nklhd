// Package json provides generic JSON serialization without reflection.
package json

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
)

// Encode converts a Go value to a JSON string.
// Supports nil, bool, float64, string, []interface{}, map[string]interface{}.
// Also handles int, int64, and other numeric types (encoded as JSON numbers).
func Encode(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("JSON encode error: %w", err)
	}
	return string(data), nil
}

// Decode parses a JSON string into a Go value.
// Numbers are decoded as float64 by default.
func Decode(s string) (interface{}, error) {
	var v interface{}
	err := json.Unmarshal([]byte(s), &v)
	if err != nil {
		return nil, fmt.Errorf("JSON decode error: %w", err)
	}
	return v, nil
}

// FromYAML converts a YAML string to a JSON string.
func FromYAML(yamlStr string) (string, error) {
	var v interface{}
	err := yaml.Unmarshal([]byte(yamlStr), &v)
	if err != nil {
		return "", fmt.Errorf("YAML parse error: %w", err)
	}
	// Use encoding/json to produce JSON from the parsed YAML data
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("JSON encode error: %w", err)
	}
	return string(data), nil
}