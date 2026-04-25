package fuse

import (
	"testing"
)

func TestMountOptionsOptionsStrings(t *testing.T) {
	tests := []struct {
		name     string
		opts     MountOptions
		expected []string
	}{
		{
			name:     "empty",
			opts:     MountOptions{},
			expected: []string{},
		},
		{
			name: "allow_other",
			opts: MountOptions{
				AllowOther: true,
			},
			expected: []string{"allow_other"},
		},
		{
			name: "default_permissions",
			opts: MountOptions{
				DefaultPermissions: true,
			},
			expected: []string{"default_permissions"},
		},
		{
			name: "extra options",
			opts: MountOptions{
				Options: []string{"max_read=65536", "allow_root"},
			},
			expected: []string{"max_read=65536", "allow_root"},
		},
		{
			name: "combined",
			opts: MountOptions{
				AllowOther:         true,
				DefaultPermissions: true,
				Options:            []string{"max_read=65536"},
			},
			expected: []string{"allow_other", "default_permissions", "max_read=65536"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.optionsStrings()
			if len(got) != len(tt.expected) {
				t.Errorf("optionsStrings() length = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("optionsStrings()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestMountOptionsOptionsStringsNoLoop(t *testing.T) {
	// Ensure we don't have the staticcheck warning about loop
	opts := MountOptions{
		Options: []string{"a", "b", "c"},
	}
	got := opts.optionsStrings()
	if len(got) != 3 {
		t.Errorf("expected 3 options, got %d", len(got))
	}
}