package wsl

import (
	"strings"
	"testing"
)

func TestBuildAST_FullExample(t *testing.T) {
	cst, err := ParseCST(fullExample(), "")
	if err != nil {
		t.Fatalf("ParseCST: %v", err)
	}
	ast, err := BuildAST(cst)
	if err != nil {
		t.Fatalf("BuildAST: %v", err)
	}

	if ast.Name != "billing.payment" {
		t.Fatalf("module: %q", ast.Name)
	}
	if len(ast.Imports) != 2 {
		t.Fatalf("imports: %d", len(ast.Imports))
	}
	if len(ast.Context) != 4 {
		t.Fatalf("context fields: %d", len(ast.Context))
	}
	if len(ast.Workflows) != 1 {
		t.Fatalf("workflows: %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Name != "charge_customer" {
		t.Fatalf("wf name: %q", wf.Name)
	}
	if wf.Start != "validate_input" {
		t.Fatalf("start: %q", wf.Start)
	}
	if len(wf.States) != 9 {
		t.Fatalf("states: %d", len(wf.States))
	}

	st := wf.States["check_balance"]
	if st == nil {
		t.Fatalf("state check_balance missing")
	}
	if st.Action == nil {
		t.Fatalf("check_balance should have action")
	}
	if st.Action.Module != "http" || st.Action.Name != "getBalance" {
		t.Fatalf("action qname: %s.%s", st.Action.Module, st.Action.Name)
	}
	if st.Action.As != "balance" {
		t.Fatalf("action as: %q", st.Action.As)
	}
	if len(st.Action.Args) != 1 {
		t.Fatalf("action args: %d", len(st.Action.Args))
	}

	fail := wf.States["fail_no_funds"]
	if fail == nil || fail.End == nil {
		t.Fatalf("terminal end missing")
	}
	if fail.End.Kind != "fail" {
		t.Fatalf("end kind: %q", fail.End.Kind)
	}
	if got := fail.End.Attr["code"]; got != "NO_FUNDS" {
		t.Fatalf("end attr code: %q", got)
	}
}

func TestBuildAST_Errors(t *testing.T) {
	// duplicate state
	src1 := `module m
workflow w { start: a
state a { on success -> b }
state a { end ok }
state b { end ok }
}`
	if _, err := BuildASTFromText(src1); err == nil || !strings.Contains(err.Error(), "duplicate state") {
		t.Fatalf("expected duplicate state error, got: %v", err)
	}

	// missing start
	src2 := `module m
workflow w { start: z
state a { end ok }
}`
	if _, err := BuildASTFromText(src2); err == nil || !strings.Contains(err.Error(), "start state 'z' not found") {
		t.Fatalf("expected missing start error, got: %v", err)
	}

	// unknown transition target
	src3 := `module m
workflow w { start: a
state a { on success -> b }
}`
	if _, err := BuildASTFromText(src3); err == nil || !strings.Contains(err.Error(), "targets unknown state") {
		t.Fatalf("expected unknown target error, got: %v", err)
	}

	// end + success transition is disallowed (ambiguous: both define what happens on success)
	src4 := `module m
workflow w { start: a
state a { on success -> a end ok }
}`
	if _, err := BuildASTFromText(src4); err == nil || !strings.Contains(err.Error(), "has both 'end' and a success transition") {
		t.Fatalf("expected end+success transition error, got: %v", err)
	}

	// end + error transition is allowed (end = success path, on error = error path)
	src5 := `module m
workflow w { start: a
state a {
  on error -> b
  end ok
}
state b { end fail }
}`
	if _, err := BuildASTFromText(src5); err != nil {
		t.Fatalf("expected no error for end+error transition, got: %v", err)
	}

	// '_' transition target resolves to next declared state
	src6 := `module m
workflow w { start: a
state a { on success -> _ }
state b { end ok }
}`
	ast6, err := BuildASTFromText(src6)
	if err != nil {
		t.Fatalf("expected no error for '_' target with next state, got: %v", err)
	}
	if len(ast6.Workflows) == 0 {
		t.Fatal("expected at least one workflow")
	}
	st6 := ast6.Workflows[0].States["a"]
	if st6 == nil {
		t.Fatal("expected state 'a' to exist")
	}
	if len(st6.Transitions) == 0 {
		t.Fatal("expected state 'a' to have at least one transition")
	}
	if got := st6.Transitions[0].Target; got != "b" {
		t.Fatalf("expected '_' to resolve to next state 'b', got: %q", got)
	}

	// trailing "on success -> _" on last state marks state as end ok
	src7 := `module m
workflow w { start: a
state a { on success -> _ }
}`
	ast7, err := BuildASTFromText(src7)
	if err != nil {
		t.Fatalf("expected trailing success '_' to be treated as final, got error: %v", err)
	}
	st7 := ast7.Workflows[0].States["a"]
	if st7.End == nil || st7.End.Kind != "ok" {
		t.Fatalf("expected trailing success '_' to set end ok, got: %+v", st7.End)
	}
	if len(st7.Transitions) != 0 {
		t.Fatalf("expected no transitions after trailing success '_' finalization, got: %d", len(st7.Transitions))
	}
}

// BuildASTFromText is a small helper for tests.
func BuildASTFromText(src string) (*Module, error) {
	cst, err := ParseCST(src, "")
	if err != nil {
		return nil, err
	}
	return BuildAST(cst)
}

// EndAtLeast is a tiny helper to placate static analysis about reading end.
func (e *End) EndAtLeast(kind string) bool { return e != nil && e.Kind == kind }
