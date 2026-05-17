package wsl

import "testing"

func TestParseAll_FullExample(t *testing.T) {
	ast, graphs, err := ParseAll(fullExample(), "")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	if ast.Name != "billing.payment" {
		t.Fatalf("module: %q", ast.Name)
	}
	if len(ast.Workflows) != 1 {
		t.Fatalf("workflows: %d", len(ast.Workflows))
	}
	if len(graphs) != 1 {
		t.Fatalf("graphs: %d", len(graphs))
	}

	g := graphs["charge_customer"]
	if g == nil {
		t.Fatalf("graph charge_customer missing")
	}
	if g.Start != "validate_input" {
		t.Fatalf("start: %q", g.Start)
	}
}

func TestParseAll_Minimal(t *testing.T) {
	src := `module m
workflow w { start: s
state s { end ok }
}`
	ast, graphs, err := ParseAll(src, "")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	if ast.Name != "m" {
		t.Fatalf("module: %q", ast.Name)
	}
	if len(ast.Workflows) != 1 {
		t.Fatalf("workflows: %d", len(ast.Workflows))
	}
	g := graphs["w"]
	if g == nil || g.Start != "s" {
		t.Fatalf("graph start: %+v", g)
	}
}
