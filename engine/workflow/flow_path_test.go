package workflow

import (
	"testing"
)

func TestParseFlowPath(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedWSL  string
		expectedFlow string
	}{
		{
			name:         "path with flow name",
			input:        "namespace/path/wsl_name.flow_name",
			expectedWSL:  "namespace/path/wsl_name",
			expectedFlow: "flow_name",
		},
		{
			name:         "simple path without flow name",
			input:        "simple_workflow",
			expectedWSL:  "simple_workflow",
			expectedFlow: "",
		},
		{
			name:         "path with multiple dots",
			input:        "namespace/path.to.wsl.flow_name",
			expectedWSL:  "namespace/path.to.wsl",
			expectedFlow: "flow_name",
		},
		{
			name:         "wsl_hello_world example",
			input:        "wsl_hello_world/example.example",
			expectedWSL:  "wsl_hello_world/example",
			expectedFlow: "example",
		},
		{
			name:         "nested path with solution",
			input:        "orchestration_with_paths/orchestration_with_paths.complete_solution",
			expectedWSL:  "orchestration_with_paths/orchestration_with_paths",
			expectedFlow: "complete_solution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wslPath, flowName := ParseFlowPath(tt.input)

			if wslPath != tt.expectedWSL {
				t.Errorf("ParseFlowPath(%q) wslPath = %q, want %q", tt.input, wslPath, tt.expectedWSL)
			}

			if flowName != tt.expectedFlow {
				t.Errorf("ParseFlowPath(%q) flowName = %q, want %q", tt.input, flowName, tt.expectedFlow)
			}
		})
	}
}
