package wsl

import "testing"

func TestBuildGraphs_FromAST(t *testing.T) {
	cst, err := ParseCST(fullExample(), "")
	if err != nil {
		t.Fatalf("ParseCST: %v", err)
	}
	ast, err := BuildAST(cst)
	if err != nil {
		t.Fatalf("BuildAST: %v", err)
	}

	graphs := BuildGraphs(ast)
	g := graphs["charge_customer"]
	if g == nil {
		t.Fatalf("graph for charge_customer missing")
	}
	if g.Start != "validate_input" {
		t.Fatalf("graph start: %q", g.Start)
	}

	if len(g.Nodes) != 9 {
		t.Fatalf("nodes: %d", len(g.Nodes))
	}

	// Check one non-terminal node
	n := g.Nodes["reserve_funds"]
	if n == nil {
		t.Fatalf("node reserve_funds missing")
	}
	if n.Terminal {
		t.Fatalf("reserve_funds should not be terminal")
	}
	if len(n.Edges) != 2 {
		t.Fatalf("reserve_funds edges: %d", len(n.Edges))
	}

	// Check terminal node attributes
	nf := g.Nodes["fail_reserve"]
	if nf == nil {
		t.Fatalf("node fail_reserve missing")
	}
	if !nf.Terminal || nf.TerminalKind != "fail" {
		t.Fatalf("fail_reserve terminal flags incorrect")
	}

	// end node without attributes (end)
	nm := g.Nodes["mark_paid"]
	if nm == nil {
		t.Fatalf("node mark_paid missing")
	}
	// it transitions to end, not terminal itself; terminal is implicit 'end' keyword modeled by End on state
}
