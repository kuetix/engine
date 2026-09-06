package wsl

import "testing"

func TestParseExpr_Valid(t *testing.T) {
	valid := []string{
		`1`, `-1`, `3.14`, `"str"`, `'str'`, `true`, `false`, `null`,
		`a`, `a.b.c`, `$constants.version`, `<<result.ok>>`,
		`a == b`, `a != b`, `a < b`, `a <= b`, `a > b`, `a >= b`,
		`a && b`, `a || b`, `!a`, `a ?? b`,
		`1 + 2 - 3`, `2 * 3 / 4 % 5`, `(1 + 2) * 3`,
		`a.b == "x" && c > 0`,
		`items[0]`, `m["key"]`, `items[i + 1]`,
		`[1, 2, 3]`, `[]`, `{a: 1, b: "x"}`, `{}`,
		`len(x)`, `contains(list, "y")`, `default(a, "z")`, `has(a.b)`,
		`"hello ${name}, ${1 + n} items"`,
		`result.status == 'completed' && result.code > 200`,
	}
	for _, src := range valid {
		if _, err := ParseExpr(src); err != nil {
			t.Errorf("ParseExpr(%q) unexpected error: %v", src, err)
		}
	}
}

func TestParseExpr_Invalid(t *testing.T) {
	invalid := []string{
		``,               // empty
		`1 +`,            // dangling operator
		`* 2`,            // leading binary operator
		`(1 + 2`,         // unbalanced paren
		`[1, 2`,          // unbalanced bracket
		`{a: 1`,          // unbalanced brace
		`{a 1}`,          // missing colon
		`"unterminated`,  // unterminated string
		`<<unterminated`, // unterminated token
		`foo(1)`,         // unknown builtin
		`1 2`,            // two primaries, no operator
		`"bad ${1 +}"`,   // bad interpolation expression
	}
	for _, src := range invalid {
		if node, err := ParseExpr(src); err == nil {
			t.Errorf("ParseExpr(%q) expected error, got node %#v", src, node)
		}
	}
}

func TestBuildAST_RejectsMalformedWhen(t *testing.T) {
	src := `
module bad

workflow bad {
  start: S
  state S {
    action a/a.Do() as r
    on success when r.ok == -> Next
    on fail -> Failed
  }
  state Next { end ok }
  state Failed { end fail }
}
`
	if _, _, err := ParseAll(src, "bad"); err == nil {
		t.Fatal("expected a build error for a malformed 'when' expression")
	}
}

func TestBuildAST_AcceptsValidWhen(t *testing.T) {
	src := `
module good

const { threshold: 10 }

workflow good {
  start: S
  state S {
    action a/a.Do() as r
    on success when r.score >= $constants.threshold && r.ok -> High
    on success -> Low
    on fail -> Failed
  }
  state High { end ok }
  state Low { end ok }
  state Failed { end fail }
}
`
	ast, _, err := ParseAll(src, "good")
	if err != nil {
		t.Fatalf("valid 'when' rejected: %v", err)
	}
	tr := ast.Workflows[0].States["S"].Transitions[0]
	if tr.WhenExpr == nil || tr.WhenExpr.Tree == nil {
		t.Fatal("expected the parsed tree to be attached to WhenExpr")
	}
}

func TestLetBindings_Parse(t *testing.T) {
	src := `
module lets

const { rate: 3 }

workflow lets {
  start: S
  state S {
    let base = 10
    let total = base * $constants.rate
    let label = "run ${base}"
    action a/a.Do(amount: total) as r
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}
`
	ast, _, err := ParseAll(src, "lets")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	s := ast.Workflows[0].States["S"]
	if len(s.Lets) != 3 {
		t.Fatalf("expected 3 let bindings, got %d", len(s.Lets))
	}
	if s.Lets[0].Name != "base" || s.Lets[1].Name != "total" || s.Lets[2].Name != "label" {
		t.Fatalf("let names: %+v", s.Lets)
	}
	for _, lb := range s.Lets {
		if lb.Expr == nil || lb.Expr.Tree == nil {
			t.Errorf("let %s: expected a parsed tree", lb.Name)
		}
	}
}

func TestLetBindings_RejectDuplicate(t *testing.T) {
	src := `
module dup
workflow dup {
  start: S
  state S {
    let x = 1
    let x = 2
    action a/a.Do() as r
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}
`
	if _, _, err := ParseAll(src, "dup"); err == nil {
		t.Fatal("expected an error for a re-assigned 'let'")
	}
}

func TestLetBindings_SWSL(t *testing.T) {
	src := `
module swlets

const { rate: 3 }

let base = 10
let total = base * $constants.rate

speak.Say(v: total) -> common.Response(message: "done") -> .
`
	ast, _, err := ParseAllSimplified(src, "swlets")
	if err != nil {
		t.Fatalf("ParseAllSimplified: %v", err)
	}
	wf := ast.Workflows[0]
	start := wf.States[wf.Start]
	if len(start.Lets) != 2 {
		t.Fatalf("expected 2 let bindings on the start state, got %d", len(start.Lets))
	}
	if start.Lets[0].Name != "base" || start.Lets[1].Name != "total" {
		t.Fatalf("let names: %+v", start.Lets)
	}
	if start.Lets[1].Expr == nil || start.Lets[1].Expr.Tree == nil {
		t.Fatal("expected parsed tree for 'total'")
	}
}

func TestLetBindings_SWSL_RejectDuplicate(t *testing.T) {
	src := `
module d
let x = 1
let x = 2
a.B() -> .
`
	if _, err := ParseSimplifiedWSL(src); err == nil {
		t.Fatal("expected an error for a re-assigned 'let' in SWSL")
	}
}

func TestLetBindings_RejectMalformed(t *testing.T) {
	src := `
module bad
workflow bad {
  start: S
  state S {
    let x = 1 +
    action a/a.Do() as r
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}
`
	if _, _, err := ParseAll(src, "bad"); err == nil {
		t.Fatal("expected an error for a malformed 'let' expression")
	}
}

func TestForEach_Parse(t *testing.T) {
	src := `
module fe
workflow fe {
  start: PostLines
  state PostLines {
    foreach line in <<invoice.lines>> {
      action ledger/ledger.Post(amount: line.amount) as posted
    }
    on success -> Done
    on fail -> Rollback
  }
  state Done { end ok }
  state Rollback { end fail }
}
`
	ast, _, err := ParseAll(src, "fe")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	s := ast.Workflows[0].States["PostLines"]
	if s.ForEach == nil {
		t.Fatal("expected ForEach on the state")
	}
	if s.ForEach.Var != "line" {
		t.Errorf("loop var = %q, want line", s.ForEach.Var)
	}
	if s.ForEach.List == nil || s.ForEach.List.Tree == nil {
		t.Error("expected a parsed tree for the collection expression")
	}
	if s.ForEach.Action == nil || s.ForEach.Action.Name != "Post" {
		t.Errorf("body action = %+v, want Post", s.ForEach.Action)
	}
	// body action is also the state's action (arg injection reuse)
	if s.Action != s.ForEach.Action {
		t.Error("state action should be the foreach body action")
	}
}

func TestForEach_ParallelParse(t *testing.T) {
	cases := []struct {
		src      string
		parallel bool
		limit    int
	}{
		{`foreach x in <<xs>> parallel[limit: 4] {`, true, 4},
		{`foreach x in <<xs>> parallel {`, true, 0},
		{`foreach x in <<xs>> {`, false, 0},
	}
	for _, c := range cases {
		src := "module m\nworkflow m {\n start: S\n state S {\n  " + c.src + `
      action a/a.Do(v: x) as r
    }
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}`
		ast, _, err := ParseAll(src, "m")
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		fe := ast.Workflows[0].States["S"].ForEach
		if fe == nil {
			t.Fatalf("%q: no ForEach", c.src)
		}
		if fe.Parallel != c.parallel || fe.ParallelLimit != c.limit {
			t.Errorf("%q: parallel=%v limit=%d, want %v/%d", c.src, fe.Parallel, fe.ParallelLimit, c.parallel, c.limit)
		}
	}
}

func TestForEach_ParallelRejectsBadLimit(t *testing.T) {
	src := `
module m
workflow m {
  start: S
  state S {
    foreach x in <<xs>> parallel[limit: 0] {
      action a/a.Do(v: x) as r
    }
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}`
	if _, _, err := ParseAll(src, "m"); err == nil {
		t.Fatal("expected an error for parallel[limit: 0]")
	}
}

func TestForEach_RejectMalformedCollection(t *testing.T) {
	src := `
module bad
workflow bad {
  start: S
  state S {
    foreach x in 1 + {
      action a/a.Do(v: x) as r
    }
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}
`
	if _, _, err := ParseAll(src, "bad"); err == nil {
		t.Fatal("expected an error for a malformed foreach collection expression")
	}
}

func TestWhile_Parse(t *testing.T) {
	src := `
module m
workflow m {
  start: S
  state S {
    while[max: 500] <<queue.size>> > 0 {
      action queue/queue.Pop() as item
    }
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}`
	ast, _, err := ParseAll(src, "m")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	wl := ast.Workflows[0].States["S"].While
	if wl == nil {
		t.Fatal("expected a while loop")
	}
	if wl.Max != 500 {
		t.Errorf("max = %d, want 500", wl.Max)
	}
	if wl.Cond == nil || wl.Cond.Tree == nil {
		t.Error("expected the condition to be parsed")
	}
	if wl.Action == nil || wl.Action.Name != "Pop" {
		t.Errorf("body action = %+v", wl.Action)
	}
}

func TestRetry_Parse(t *testing.T) {
	src := `
module m
workflow m {
  start: S
  state S {
    retry[max: 3, delay: "200ms", on: "err.retryable == true"]
    action pay/pay.Charge() as ch
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}`
	ast, _, err := ParseAll(src, "m")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	rp := ast.Workflows[0].States["S"].Retry
	if rp == nil {
		t.Fatal("expected a retry policy")
	}
	if rp.Max != 3 || rp.Delay != "200ms" {
		t.Errorf("retry = %+v", rp)
	}
	if rp.On == nil || rp.On.Tree == nil {
		t.Error("expected the 'on' expression to be parsed")
	}
}

func TestRetry_ParseMinimal(t *testing.T) {
	src := `
module m
workflow m {
  start: S
  state S {
    retry[max: 1]
    action a/a.Do() as r
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}`
	ast, _, err := ParseAll(src, "m")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	rp := ast.Workflows[0].States["S"].Retry
	if rp == nil || rp.Max != 1 || rp.Delay != "" || rp.On != nil {
		t.Errorf("minimal retry = %+v", rp)
	}
}

func TestParseExpr_Shape(t *testing.T) {
	// precedence: 1 + 2 * 3  =>  (+ 1 (* 2 3))
	n, err := ParseExpr(`1 + 2 * 3`)
	if err != nil {
		t.Fatal(err)
	}
	bin, ok := n.(*BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("root: got %#v, want BinaryExpr '+'", n)
	}
	rhs, ok := bin.R.(*BinaryExpr)
	if !ok || rhs.Op != "*" {
		t.Fatalf("rhs: got %#v, want BinaryExpr '*'", bin.R)
	}

	// && binds looser than == :  a == b && c == d  =>  (&& (== a b) (== c d))
	n2, _ := ParseExpr(`a == b && c == d`)
	and, ok := n2.(*BinaryExpr)
	if !ok || and.Op != "&&" {
		t.Fatalf("root: got %#v, want '&&'", n2)
	}
	if l, ok := and.L.(*BinaryExpr); !ok || l.Op != "==" {
		t.Fatalf("&& left: got %#v, want '=='", and.L)
	}

	// <<x>> lowers to PathExpr
	n3, _ := ParseExpr(`<<a.b>>`)
	if pe, ok := n3.(*PathExpr); !ok || pe.Path != "a.b" {
		t.Fatalf("<<a.b>>: got %#v, want PathExpr a.b", n3)
	}

	// interpolation lowers to InterpExpr
	n4, _ := ParseExpr(`"x${y}z"`)
	if _, ok := n4.(*InterpExpr); !ok {
		t.Fatalf(`"x${y}z": got %#v, want InterpExpr`, n4)
	}

	// plain string stays LitExpr
	n5, _ := ParseExpr(`"plain"`)
	if l, ok := n5.(*LitExpr); !ok || l.Kind != "string" || l.Str != "plain" {
		t.Fatalf(`"plain": got %#v`, n5)
	}
}
