// Package yaml provides generic YAML serialization using yaml.v3.
package yaml

import (
	"gopkg.in/yaml.v3"
)

// Encode converts a Go value to a YAML string using yaml.v3.
func Encode(v interface{}) (string, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	// Remove trailing newline added by yaml.Marshal
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	return string(data), nil
}

// Decode parses a YAML string into a Go value using yaml.v3.
func Decode(s string) (interface{}, error) {
	var v interface{}
	err := yaml.Unmarshal([]byte(s), &v)
	return v, err
}

// FromJSON converts a JSON string to a YAML string.
func FromJSON(jsonStr string) (string, error) {
	var v interface{}
	err := yaml.Unmarshal([]byte(jsonStr), &v)
	if err != nil {
		return "", err
	}
	return Encode(v)
}


