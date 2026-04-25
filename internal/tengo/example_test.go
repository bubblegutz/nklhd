package tengo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingExamples(t *testing.T) {
	examples := []struct{
		name string
		path string
	}{
		{"simpler", "simpler/main.tengo"},
		{"utilities", "utilities/main.tengo"},
		{"serialization", "serialization/main.tengo"},
		{"conversion", "conversion/main.tengo"},
		{"http", "http/main.tengo"},
		{"jq", "jq/main.tengo"},
		{"oauth", "oauth/main.tengo"},
		{"simple", "simple/main.tengo"},
		{"wikipedia", "wikipedia/main.tengo"},
	}
	base := "../../examples/tengo/"
	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			absPath, err := filepath.Abs(base + ex.path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				t.Skipf("example not found: %s", absPath)
			}
			exec := NewExecutor()
			defer exec.Close()
			if err := exec.LoadScript(absPath); err != nil {
				t.Fatalf("failed to load %s: %v", ex.name, err)
			}
		})
	}
}
