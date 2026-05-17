package workflow

import (
	"testing"
)

func TestStripWSLExtension(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"wsl_hello_world/example.wsl", "example"},
		{"example.wsl", "example"},
		{"example", "example"},
		{"path/to/workflow.wsl", "workflow"},
		{"path/to/workflow", "workflow"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			name := tc.input
			want := name

			// Extract last segment after /
			if idx := len(name) - 1; idx >= 0 {
				for i := idx; i >= 0; i-- {
					if name[i] == '/' {
						want = name[i+1:]
						break
					}
				}
			}

			// Strip .wsl extension if present
			if len(want) > 4 && want[len(want)-4:] == ".wsl" {
				want = want[:len(want)-4]
			}

			if want != tc.expected {
				t.Errorf("For input '%s': expected '%s', got '%s'", tc.input, tc.expected, want)
			}
		})
	}
}
