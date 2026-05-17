package wsl

import (
	"testing"
)

func TestParseCST_CustomWorkflowType(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		expectedType string
		expectError  bool
	}{
		{
			name: "feature type",
			src: `
module example

feature example {
  start: CheckSystem

  state CheckSystem {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`,
			expectedType: "feature",
			expectError:  false,
		},
		{
			name: "solution type",
			src: `
module example

solution example {
  start: CheckSystem

  state CheckSystem {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`,
			expectedType: "solution",
			expectError:  false,
		},
		{
			name: "workflow type (original)",
			src: `
module example

workflow example {
  start: CheckSystem

  state CheckSystem {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`,
			expectedType: "workflow",
			expectError:  false,
		},
		{
			name: "custom type",
			src: `
module example

custom_type example {
  start: CheckSystem

  state CheckSystem {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`,
			expectedType: "custom_type",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cst, err := ParseCST(tt.src, "")
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCST failed: %v", err)
			}

			if len(cst.Workflows) != 1 {
				t.Fatalf("expected 1 workflow, got %d", len(cst.Workflows))
			}

			wf := cst.Workflows[0]
			if wf.TypeTok.Lexeme != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, wf.TypeTok.Lexeme)
			}
		})
	}
}

func TestBuildAST_CustomWorkflowType(t *testing.T) {
	src := `
module example

feature myfeature {
  start: Init

  state Init {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`
	cst, err := ParseCST(src, "")
	if err != nil {
		t.Fatalf("ParseCST failed: %v", err)
	}

	ast, err := BuildAST(cst)
	if err != nil {
		t.Fatalf("BuildAST failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "feature" {
		t.Errorf("expected workflow type 'feature', got %q", wf.Type)
	}
	if wf.Name != "myfeature" {
		t.Errorf("expected workflow name 'myfeature', got %q", wf.Name)
	}
}

func TestBuildGraph_CustomWorkflowType(t *testing.T) {
	src := `
module example

solution myapp {
  start: Init

  state Init {
    action services/common/response.ResponseValue(value: "test", statusCode: 200)
    end ok
  }
}
`
	cst, err := ParseCST(src, "")
	if err != nil {
		t.Fatalf("ParseCST failed: %v", err)
	}

	ast, err := BuildAST(cst)
	if err != nil {
		t.Fatalf("BuildAST failed: %v", err)
	}

	graphs := BuildGraphs(ast)
	if len(graphs) != 1 {
		t.Fatalf("expected 1 graph, got %d", len(graphs))
	}

	graph := graphs["myapp"]
	if graph == nil {
		t.Fatalf("graph 'myapp' not found")
	}

	if graph.WorkflowType != "solution" {
		t.Errorf("expected workflow type 'solution', got %q", graph.WorkflowType)
	}
	if graph.WorkflowName != "myapp" {
		t.Errorf("expected workflow name 'myapp', got %q", graph.WorkflowName)
	}
}
