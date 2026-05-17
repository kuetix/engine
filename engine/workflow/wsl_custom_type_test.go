package workflow

import (
	"testing"

	"github.com/kuetix/engine/internal/wsl"
)

func TestWslGraphToSchema_CustomWorkflowType(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		workflowName string
		expectedType string
	}{
		{
			name: "project type",
			src: `
module example

project myproject {
  start: Init

  state Init {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`,
			workflowName: "myproject",
			expectedType: "project",
		},
		{
			name: "application type",
			src: `
module example

application myapp {
  start: Init

  state Init {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`,
			workflowName: "myapp",
			expectedType: "application",
		},
		{
			name: "workflow type (original)",
			src: `
module example

workflow standard_workflow {
  start: Init

  state Init {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`,
			workflowName: "standard_workflow",
			expectedType: "workflow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, graphs, err := wsl.ParseAll(tt.src, "")
			if err != nil {
				t.Fatalf("ParseAll failed: %v", err)
			}

			if ast == nil {
				t.Fatalf("AST is nil")
			}

			graph, ok := graphs[tt.workflowName]
			if !ok {
				t.Fatalf("workflow %q not found in graphs", tt.workflowName)
			}

			schema := wslGraphToSchema(graph)

			// Check if type is in schema
			typeVal, hasType := schema["type"]
			if !hasType {
				t.Errorf("schema does not have 'type' field")
			}

			typeStr, ok := typeVal.(string)
			if !ok {
				t.Errorf("type field is not a string: %T", typeVal)
			}

			if typeStr != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, typeStr)
			}
		})
	}
}
