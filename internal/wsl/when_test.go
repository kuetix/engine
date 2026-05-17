package wsl

import (
	"testing"
)

func TestParseWhenExpression(t *testing.T) {
	src := `
module test

workflow test_when {
  start: CheckVersion

  state CheckVersion {
    action services/common/response.ResponseValue(value: "test", statusCode: 200) as Result
    on success when $constants.version == "1.0.0" -> VersionOne
    on success when $constants.version == "2.0.0" -> VersionTwo
    on success -> Default
    on fail when $constants.retry > 0 -> Retry
    on fail -> Error
  }

  state VersionOne {
    end ok
  }

  state VersionTwo {
    end ok
  }

  state Default {
    end ok
  }

  state Retry {
    end ok
  }

  state Error {
    end fail
  }
}
`

	// Parse CST
	cst, err := ParseCST(src, "")
	if err != nil {
		t.Fatalf("ParseCST failed: %v", err)
	}

	if len(cst.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(cst.Workflows))
	}

	wf := cst.Workflows[0]
	if len(wf.States) != 6 {
		t.Fatalf("expected 6 states, got %d", len(wf.States))
	}

	// Check CheckVersion state
	checkState := wf.States[0]
	if checkState.NameTok.Lexeme != "CheckVersion" {
		t.Fatalf("expected state 'CheckVersion', got '%s'", checkState.NameTok.Lexeme)
	}

	if len(checkState.Transitions) != 5 {
		t.Fatalf("expected 5 transitions, got %d", len(checkState.Transitions))
	}

	// Check first transition: on success when $constants.version == "1.0.0" -> VersionOne
	tr1 := checkState.Transitions[0]
	if tr1.Cond.Kind != TokSuccess {
		t.Errorf("transition 1: expected success condition, got %v", tr1.Cond.Kind)
	}
	if tr1.Cond.WhenExpr == nil {
		t.Error("transition 1: expected when expression, got nil")
	} else if tr1.Cond.WhenExpr.Raw != `$constants.version = = "1.0.0"` {
		t.Errorf("transition 1: expected when expression '$constants.version = = \"1.0.0\"', got '%s'", tr1.Cond.WhenExpr.Raw)
	}
	if tr1.TargetTok.Lexeme != "VersionOne" {
		t.Errorf("transition 1: expected target 'VersionOne', got '%s'", tr1.TargetTok.Lexeme)
	}

	// Check second transition: on success when $constants.version == "2.0.0" -> VersionTwo
	tr2 := checkState.Transitions[1]
	if tr2.Cond.Kind != TokSuccess {
		t.Errorf("transition 2: expected success condition, got %v", tr2.Cond.Kind)
	}
	if tr2.Cond.WhenExpr == nil {
		t.Error("transition 2: expected when expression, got nil")
	} else if tr2.Cond.WhenExpr.Raw != `$constants.version = = "2.0.0"` {
		t.Errorf("transition 2: expected when expression '$constants.version = = \"2.0.0\"', got '%s'", tr2.Cond.WhenExpr.Raw)
	}

	// Check third transition: on success -> Default (no when)
	tr3 := checkState.Transitions[2]
	if tr3.Cond.Kind != TokSuccess {
		t.Errorf("transition 3: expected success condition, got %v", tr3.Cond.Kind)
	}
	if tr3.Cond.WhenExpr != nil {
		t.Errorf("transition 3: expected no when expression, got '%v'", tr3.Cond.WhenExpr)
	}

	// Check fourth transition: on fail when $constants.retry > 0 -> Retry
	tr4 := checkState.Transitions[3]
	if tr4.Cond.Kind != TokFail {
		t.Errorf("transition 4: expected fail condition, got %v", tr4.Cond.Kind)
	}
	if tr4.Cond.WhenExpr == nil {
		t.Error("transition 4: expected when expression, got nil")
	} else if tr4.Cond.WhenExpr.Raw != `$constants.retry > 0` {
		t.Errorf("transition 4: expected when expression '$constants.retry > 0', got '%s'", tr4.Cond.WhenExpr.Raw)
	}

	// Build AST
	ast, err := BuildAST(cst)
	if err != nil {
		t.Fatalf("BuildAST failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("AST: expected 1 workflow, got %d", len(ast.Workflows))
	}

	astWf := ast.Workflows[0]
	checkStateAST := astWf.States["CheckVersion"]
	if checkStateAST == nil {
		t.Fatal("AST: CheckVersion state not found")
	}

	if len(checkStateAST.Transitions) != 5 {
		t.Fatalf("AST: expected 5 transitions, got %d", len(checkStateAST.Transitions))
	}

	// Verify when expressions are preserved in AST
	astTr1 := checkStateAST.Transitions[0]
	if astTr1.WhenExpr == nil {
		t.Error("AST transition 1: expected when expression, got nil")
	} else if astTr1.WhenExpr.Raw != `$constants.version = = "1.0.0"` {
		t.Errorf("AST transition 1: expected when expression '$constants.version = = \"1.0.0\"', got '%s'", astTr1.WhenExpr.Raw)
	}

	astTr3 := checkStateAST.Transitions[2]
	if astTr3.WhenExpr != nil {
		t.Errorf("AST transition 3: expected no when expression, got '%v'", astTr3.WhenExpr)
	}

	// Build Graph
	graph := BuildGraph(astWf)
	if graph == nil {
		t.Fatal("BuildGraph returned nil")
	}

	checkNode := graph.Nodes["CheckVersion"]
	if checkNode == nil {
		t.Fatal("Graph: CheckVersion node not found")
	}

	if len(checkNode.Edges) != 5 {
		t.Fatalf("Graph: expected 5 edges, got %d", len(checkNode.Edges))
	}

	// Verify when expressions are preserved in graph edges
	edge1 := checkNode.Edges[0]
	if edge1.WhenExpr == nil {
		t.Error("Graph edge 1: expected when expression, got nil")
	} else if edge1.WhenExpr.Raw != `$constants.version = = "1.0.0"` {
		t.Errorf("Graph edge 1: expected when expression '$constants.version = = \"1.0.0\"', got '%s'", edge1.WhenExpr.Raw)
	}

	edge3 := checkNode.Edges[2]
	if edge3.WhenExpr != nil {
		t.Errorf("Graph edge 3: expected no when expression, got '%v'", edge3.WhenExpr)
	}
}
