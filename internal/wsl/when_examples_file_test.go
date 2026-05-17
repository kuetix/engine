package wsl

import (
	"os"
	"testing"
)

// TestWhenExamplesFile_FullGraph is a comprehensive test for
// runtime/workflows/wsl_hello_world/when_examples.wsl.
// It exercises the complete CST → AST → Graph pipeline and verifies
// every node, every edge, all when-expression raw values, state params,
// terminal state, and edge targets/args.
//
// The WSL lexer tokenises multi-character comparison operators as individual
// characters and the expression builder joins them with spaces, so the raw
// form of "==" is "= =" and "<=" is "< =".  This is intentional and
// consistent with the assertions in when_test.go.
func TestWhenExamplesFile_FullGraph(t *testing.T) {
	src, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/when_examples.wsl")
	if err != nil {
		t.Fatalf("failed to read when_examples.wsl: %v", err)
	}

	// ── CST ──────────────────────────────────────────────────────────────────
	cst, err := ParseCST(string(src), "")
	if err != nil {
		t.Fatalf("ParseCST failed: %v", err)
	}

	if cst.NameTok.Lexeme != "when_examples" {
		t.Errorf("CST: module name = %q, want %q", cst.NameTok.Lexeme, "when_examples")
	}

	if len(cst.Workflows) != 1 {
		t.Fatalf("CST: expected 1 workflow, got %d", len(cst.Workflows))
	}

	cstWf := cst.Workflows[0]
	if cstWf.NameTok.Lexeme != "conditional_example" {
		t.Errorf("CST: workflow name = %q, want %q", cstWf.NameTok.Lexeme, "conditional_example")
	}
	if cstWf.StartName.Lexeme != "CheckVersion" {
		t.Errorf("CST: start = %q, want %q", cstWf.StartName.Lexeme, "CheckVersion")
	}
	if len(cstWf.States) != 9 {
		t.Fatalf("CST: expected 9 states, got %d", len(cstWf.States))
	}

	// CST const block
	if cst.Constants == nil {
		t.Fatal("CST: expected const block, got nil")
	}
	if len(cst.Constants.Entries) != 3 {
		t.Errorf("CST: expected 3 const entries, got %d", len(cst.Constants.Entries))
	}

	// Spot-check CheckVersion state at CST level
	var cstCheck *CSTState
	for i := range cstWf.States {
		if cstWf.States[i].NameTok.Lexeme == "CheckVersion" {
			cstCheck = &cstWf.States[i]
			break
		}
	}
	if cstCheck == nil {
		t.Fatal("CST: state 'CheckVersion' not found")
	}
	if len(cstCheck.Transitions) != 3 {
		t.Fatalf("CST: CheckVersion should have 3 transitions, got %d", len(cstCheck.Transitions))
	}
	// First transition has a when expression
	if cstCheck.Transitions[0].Cond.WhenExpr == nil {
		t.Error("CST: CheckVersion transition[0] should have a when expression")
	}
	// Third transition is a plain 'on success' fallback (no when)
	if cstCheck.Transitions[2].Cond.WhenExpr != nil {
		t.Error("CST: CheckVersion transition[2] should not have a when expression")
	}

	// ── AST ──────────────────────────────────────────────────────────────────
	ast, err := BuildAST(cst)
	if err != nil {
		t.Fatalf("BuildAST failed: %v", err)
	}

	if ast.Name != "when_examples" {
		t.Errorf("AST: module name = %q, want %q", ast.Name, "when_examples")
	}
	if len(ast.Workflows) != 1 {
		t.Fatalf("AST: expected 1 workflow, got %d", len(ast.Workflows))
	}
	if len(ast.Constants) != 3 {
		t.Errorf("AST: expected 3 constants, got %d", len(ast.Constants))
	}

	astWf := ast.Workflows[0]
	if len(astWf.States) != 9 {
		t.Fatalf("AST: expected 9 states, got %d", len(astWf.States))
	}

	// AST CheckVersion: when expressions preserved
	astCheck := astWf.States["CheckVersion"]
	if astCheck == nil {
		t.Fatal("AST: state 'CheckVersion' not found")
	}
	if len(astCheck.Transitions) != 3 {
		t.Fatalf("AST: CheckVersion should have 3 transitions, got %d", len(astCheck.Transitions))
	}
	if astCheck.Transitions[0].WhenExpr == nil {
		t.Error("AST: CheckVersion transition[0] should have a when expression")
	} else if got := astCheck.Transitions[0].WhenExpr.Raw; got != `$constants.version = = "1.0.0"` {
		t.Errorf("AST: CheckVersion transition[0] when = %q, want %q", got, `$constants.version = = "1.0.0"`)
	}
	if astCheck.Transitions[1].WhenExpr == nil {
		t.Error("AST: CheckVersion transition[1] should have a when expression")
	} else if got := astCheck.Transitions[1].WhenExpr.Raw; got != `$constants.version = = "2.0.0"` {
		t.Errorf("AST: CheckVersion transition[1] when = %q, want %q", got, `$constants.version = = "2.0.0"`)
	}
	if astCheck.Transitions[2].WhenExpr != nil {
		t.Error("AST: CheckVersion transition[2] should not have a when expression")
	}

	// AST VersionOneHandler: two when expressions
	astV1 := astWf.States["VersionOneHandler"]
	if astV1 == nil {
		t.Fatal("AST: state 'VersionOneHandler' not found")
	}
	if len(astV1.Transitions) != 2 {
		t.Fatalf("AST: VersionOneHandler should have 2 transitions, got %d", len(astV1.Transitions))
	}
	if astV1.Transitions[0].WhenExpr == nil {
		t.Error("AST: VersionOneHandler transition[0] should have a when expression")
	} else if got := astV1.Transitions[0].WhenExpr.Raw; got != `$constants.enabled = = true` {
		t.Errorf("AST: VersionOneHandler transition[0] when = %q, want %q", got, `$constants.enabled = = true`)
	}
	if astV1.Transitions[1].WhenExpr == nil {
		t.Error("AST: VersionOneHandler transition[1] should have a when expression")
	} else if got := astV1.Transitions[1].WhenExpr.Raw; got != `$constants.enabled = = false` {
		t.Errorf("AST: VersionOneHandler transition[1] when = %q, want %q", got, `$constants.enabled = = false`)
	}

	// AST ProcessEnabled: when expressions with comparison operators
	astPE := astWf.States["ProcessEnabled"]
	if astPE == nil {
		t.Fatal("AST: state 'ProcessEnabled' not found")
	}
	if len(astPE.Transitions) != 2 {
		t.Fatalf("AST: ProcessEnabled should have 2 transitions, got %d", len(astPE.Transitions))
	}
	if astPE.Transitions[0].WhenExpr == nil {
		t.Error("AST: ProcessEnabled transition[0] should have a when expression")
	} else if got := astPE.Transitions[0].WhenExpr.Raw; got != `$constants.maxRetries > 2` {
		t.Errorf("AST: ProcessEnabled transition[0] when = %q, want %q", got, `$constants.maxRetries > 2`)
	}
	if astPE.Transitions[1].WhenExpr == nil {
		t.Error("AST: ProcessEnabled transition[1] should have a when expression")
	} else if got := astPE.Transitions[1].WhenExpr.Raw; got != `$constants.maxRetries < = 2` {
		t.Errorf("AST: ProcessEnabled transition[1] when = %q, want %q", got, `$constants.maxRetries < = 2`)
	}

	// AST FinalState: terminal, params
	astFinal := astWf.States["FinalState"]
	if astFinal == nil {
		t.Fatal("AST: state 'FinalState' not found")
	}
	if len(astFinal.Params) != 1 || astFinal.Params[0] != "Result" {
		t.Errorf("AST: FinalState params = %v, want [Result]", astFinal.Params)
	}
	if astFinal.End == nil {
		t.Fatal("AST: FinalState should have end")
	}
	if astFinal.End.Kind != "ok" {
		t.Errorf("AST: FinalState end kind = %q, want %q", astFinal.End.Kind, "ok")
	}

	// AST HighRetryPath / LowRetryPath: state params
	astHR := astWf.States["HighRetryPath"]
	if astHR == nil {
		t.Fatal("AST: state 'HighRetryPath' not found")
	}
	if len(astHR.Params) != 1 || astHR.Params[0] != "EnabledResult" {
		t.Errorf("AST: HighRetryPath params = %v, want [EnabledResult]", astHR.Params)
	}
	astLR := astWf.States["LowRetryPath"]
	if astLR == nil {
		t.Fatal("AST: state 'LowRetryPath' not found")
	}
	if len(astLR.Params) != 1 || astLR.Params[0] != "EnabledResult" {
		t.Errorf("AST: LowRetryPath params = %v, want [EnabledResult]", astLR.Params)
	}

	// ── Graph ─────────────────────────────────────────────────────────────────
	graph := BuildGraph(astWf)
	if graph == nil {
		t.Fatal("BuildGraph returned nil")
	}

	if graph.WorkflowName != "conditional_example" {
		t.Errorf("Graph: WorkflowName = %q, want %q", graph.WorkflowName, "conditional_example")
	}
	if graph.WorkflowType != "workflow" {
		t.Errorf("Graph: WorkflowType = %q, want %q", graph.WorkflowType, "workflow")
	}
	if graph.Start != "CheckVersion" {
		t.Errorf("Graph: Start = %q, want %q", graph.Start, "CheckVersion")
	}
	if len(graph.Nodes) != 9 {
		t.Fatalf("Graph: expected 9 nodes, got %d", len(graph.Nodes))
	}

	// CheckVersion
	checkNode := graph.Nodes["CheckVersion"]
	if checkNode == nil {
		t.Fatal("Graph: node 'CheckVersion' not found")
	}
	if len(checkNode.Edges) != 3 {
		t.Fatalf("Graph: CheckVersion expected 3 edges, got %d", len(checkNode.Edges))
	}
	assertEdge(t, "CheckVersion", 0, checkNode.Edges[0], "success", `$constants.version = = "1.0.0"`, "VersionOneHandler")
	assertEdge(t, "CheckVersion", 1, checkNode.Edges[1], "success", `$constants.version = = "2.0.0"`, "VersionTwoHandler")
	assertEdgeNoWhen(t, "CheckVersion", 2, checkNode.Edges[2], "success", "DefaultHandler")

	// VersionOneHandler
	v1Node := graph.Nodes["VersionOneHandler"]
	if v1Node == nil {
		t.Fatal("Graph: node 'VersionOneHandler' not found")
	}
	if len(v1Node.Edges) != 2 {
		t.Fatalf("Graph: VersionOneHandler expected 2 edges, got %d", len(v1Node.Edges))
	}
	assertEdge(t, "VersionOneHandler", 0, v1Node.Edges[0], "success", `$constants.enabled = = true`, "ProcessEnabled")
	assertEdge(t, "VersionOneHandler", 1, v1Node.Edges[1], "success", `$constants.enabled = = false`, "ProcessDisabled")

	// VersionTwoHandler
	v2Node := graph.Nodes["VersionTwoHandler"]
	if v2Node == nil {
		t.Fatal("Graph: node 'VersionTwoHandler' not found")
	}
	if len(v2Node.Edges) != 1 {
		t.Fatalf("Graph: VersionTwoHandler expected 1 edge, got %d", len(v2Node.Edges))
	}
	assertEdgeNoWhen(t, "VersionTwoHandler", 0, v2Node.Edges[0], "success", "FinalState")

	// DefaultHandler
	dhNode := graph.Nodes["DefaultHandler"]
	if dhNode == nil {
		t.Fatal("Graph: node 'DefaultHandler' not found")
	}
	if len(dhNode.Edges) != 1 {
		t.Fatalf("Graph: DefaultHandler expected 1 edge, got %d", len(dhNode.Edges))
	}
	assertEdgeNoWhen(t, "DefaultHandler", 0, dhNode.Edges[0], "success", "FinalState")

	// ProcessEnabled
	peNode := graph.Nodes["ProcessEnabled"]
	if peNode == nil {
		t.Fatal("Graph: node 'ProcessEnabled' not found")
	}
	if len(peNode.Edges) != 2 {
		t.Fatalf("Graph: ProcessEnabled expected 2 edges, got %d", len(peNode.Edges))
	}
	assertEdge(t, "ProcessEnabled", 0, peNode.Edges[0], "success", `$constants.maxRetries > 2`, "HighRetryPath")
	assertEdge(t, "ProcessEnabled", 1, peNode.Edges[1], "success", `$constants.maxRetries < = 2`, "LowRetryPath")

	// ProcessDisabled
	pdNode := graph.Nodes["ProcessDisabled"]
	if pdNode == nil {
		t.Fatal("Graph: node 'ProcessDisabled' not found")
	}
	if len(pdNode.Edges) != 1 {
		t.Fatalf("Graph: ProcessDisabled expected 1 edge, got %d", len(pdNode.Edges))
	}
	assertEdgeNoWhen(t, "ProcessDisabled", 0, pdNode.Edges[0], "success", "FinalState")

	// HighRetryPath
	hrNode := graph.Nodes["HighRetryPath"]
	if hrNode == nil {
		t.Fatal("Graph: node 'HighRetryPath' not found")
	}
	if len(hrNode.ParamNames) != 1 || hrNode.ParamNames[0] != "EnabledResult" {
		t.Errorf("Graph: HighRetryPath params = %v, want [EnabledResult]", hrNode.ParamNames)
	}
	if len(hrNode.Edges) != 1 {
		t.Fatalf("Graph: HighRetryPath expected 1 edge, got %d", len(hrNode.Edges))
	}
	assertEdgeNoWhen(t, "HighRetryPath", 0, hrNode.Edges[0], "success", "FinalState")

	// LowRetryPath
	lrNode := graph.Nodes["LowRetryPath"]
	if lrNode == nil {
		t.Fatal("Graph: node 'LowRetryPath' not found")
	}
	if len(lrNode.ParamNames) != 1 || lrNode.ParamNames[0] != "EnabledResult" {
		t.Errorf("Graph: LowRetryPath params = %v, want [EnabledResult]", lrNode.ParamNames)
	}
	if len(lrNode.Edges) != 1 {
		t.Fatalf("Graph: LowRetryPath expected 1 edge, got %d", len(lrNode.Edges))
	}
	assertEdgeNoWhen(t, "LowRetryPath", 0, lrNode.Edges[0], "success", "FinalState")

	// FinalState: terminal, params
	fsNode := graph.Nodes["FinalState"]
	if fsNode == nil {
		t.Fatal("Graph: node 'FinalState' not found")
	}
	if len(fsNode.ParamNames) != 1 || fsNode.ParamNames[0] != "Result" {
		t.Errorf("Graph: FinalState params = %v, want [Result]", fsNode.ParamNames)
	}
	if !fsNode.Terminal {
		t.Error("Graph: FinalState should be terminal")
	}
	if fsNode.TerminalKind != "ok" {
		t.Errorf("Graph: FinalState TerminalKind = %q, want %q", fsNode.TerminalKind, "ok")
	}
	if len(fsNode.Edges) != 0 {
		t.Errorf("Graph: FinalState should have no edges, got %d", len(fsNode.Edges))
	}
}

// assertEdge checks an edge that must carry a when expression.
func assertEdge(t *testing.T, node string, idx int, e Edge, wantCond, wantWhen, wantTo string) {
	t.Helper()
	if e.Condition.Kind != wantCond {
		t.Errorf("Graph: %s edge[%d] condition = %q, want %q", node, idx, e.Condition.Kind, wantCond)
	}
	if e.WhenExpr == nil {
		t.Errorf("Graph: %s edge[%d] should have a when expression", node, idx)
	} else if e.WhenExpr.Raw != wantWhen {
		t.Errorf("Graph: %s edge[%d] when = %q, want %q", node, idx, e.WhenExpr.Raw, wantWhen)
	}
	if e.To != wantTo {
		t.Errorf("Graph: %s edge[%d] to = %q, want %q", node, idx, e.To, wantTo)
	}
}

// assertEdgeNoWhen checks an edge that must not carry a when expression.
func assertEdgeNoWhen(t *testing.T, node string, idx int, e Edge, wantCond, wantTo string) {
	t.Helper()
	if e.Condition.Kind != wantCond {
		t.Errorf("Graph: %s edge[%d] condition = %q, want %q", node, idx, e.Condition.Kind, wantCond)
	}
	if e.WhenExpr != nil {
		t.Errorf("Graph: %s edge[%d] should not have a when expression, got %q", node, idx, e.WhenExpr.Raw)
	}
	if e.To != wantTo {
		t.Errorf("Graph: %s edge[%d] to = %q, want %q", node, idx, e.To, wantTo)
	}
}
