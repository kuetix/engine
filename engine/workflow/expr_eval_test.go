package workflow

import (
	"math"
	"testing"

	"github.com/kuetix/engine/internal/wsl"
)

// mapScope is a trivial ExprScope for evaluator tests.
type mapScope map[string]interface{}

func (m mapScope) ResolvePath(path string) (interface{}, bool) {
	v, ok := m[path]
	return v, ok
}

func evalStr(t *testing.T, src string, scope ExprScope) (interface{}, error) {
	t.Helper()
	node, err := wsl.ParseExpr(src)
	if err != nil {
		return nil, err
	}
	return EvalExpr(node, scope)
}

func mustEval(t *testing.T, src string, scope ExprScope) interface{} {
	t.Helper()
	v, err := evalStr(t, src, scope)
	if err != nil {
		t.Fatalf("eval %q: unexpected error: %v", src, err)
	}
	return v
}

func TestEval_Literals(t *testing.T) {
	s := mapScope{}
	cases := []struct {
		src  string
		want interface{}
	}{
		{`1`, int64(1)},
		{`-1`, int64(-1)},
		{`3.5`, 3.5},
		{`"hi"`, "hi"},
		{`'hi'`, "hi"},
		{`true`, true},
		{`false`, false},
		{`null`, nil},
		{`"a\nb"`, "a\nb"},
		{`1_000`, int64(1000)},
	}
	for _, c := range cases {
		if got := mustEval(t, c.src, s); got != c.want {
			t.Errorf("%q = %#v, want %#v", c.src, got, c.want)
		}
	}
}

func TestEval_Arithmetic(t *testing.T) {
	s := mapScope{}
	cases := []struct {
		src  string
		want interface{}
	}{
		{`2 + 3`, int64(5)},
		{`2 - 5`, int64(-3)},
		{`4 * 3`, int64(12)},
		{`10 / 2`, int64(5)},
		{`7 / 2`, 3.5}, // non-even int division promotes to float
		{`7 % 3`, int64(1)},
		{`2 + 3 * 4`, int64(14)}, // precedence
		{`(2 + 3) * 4`, int64(20)},
		{`1.5 + 1`, 2.5},
		{`10 - 2 - 3`, int64(5)}, // left assoc
		{`2 * 3.0`, 6.0},
	}
	for _, c := range cases {
		if got := mustEval(t, c.src, s); got != c.want {
			t.Errorf("%q = %#v, want %#v", c.src, got, c.want)
		}
	}
}

func TestEval_ArithmeticErrors(t *testing.T) {
	s := mapScope{}
	for _, src := range []string{`1 / 0`, `5 % 0`, `"a" - 1`, `true * 2`, `null + 1`} {
		if v, err := evalStr(t, src, s); err == nil {
			t.Errorf("%q: expected error, got %#v", src, v)
		}
	}
}

func TestEval_StringConcatAndInterp(t *testing.T) {
	s := mapScope{"user.name": "Ann", "n": int64(3)}
	if got := mustEval(t, `"a" + "b"`, s); got != "ab" {
		t.Errorf(`"a"+"b" = %#v`, got)
	}
	if _, err := evalStr(t, `"a" + 1`, s); err == nil {
		t.Error(`"a" + 1 should error (use interpolation)`)
	}
	if got := mustEval(t, `"hi ${user.name}, ${n} left"`, s); got != "hi Ann, 3 left" {
		t.Errorf("interp = %#v", got)
	}
	if got := mustEval(t, `"${1 + 2}"`, s); got != "3" {
		t.Errorf("interp expr = %#v", got)
	}
}

func TestEval_Comparisons(t *testing.T) {
	s := mapScope{}
	cases := []struct {
		src  string
		want bool
	}{
		{`1 == 1`, true},
		{`1 == 1.0`, true}, // numeric cross-type
		{`1 != 2`, true},
		{`"a" == "a"`, true},
		{`"a" == "b"`, false},
		{`true == true`, true},
		{`null == null`, true},
		{`1 < 2`, true},
		{`2 <= 2`, true},
		{`3 > 2`, true},
		{`3 >= 4`, false},
		{`"a" < "b"`, true},
		{`[1,2] == [1,2]`, true},
		{`[1,2] == [1,3]`, false},
	}
	for _, c := range cases {
		if got := mustEval(t, c.src, s); got != c.want {
			t.Errorf("%q = %#v, want %v", c.src, got, c.want)
		}
	}
}

func TestEval_ComparisonTypeErrors(t *testing.T) {
	s := mapScope{}
	for _, src := range []string{`"1" < 2`, `true < false`, `null < 1`} {
		if v, err := evalStr(t, src, s); err == nil {
			t.Errorf("%q: expected error, got %#v", src, v)
		}
	}
	// cross-type equality is defined (not an error), just false
	if got := mustEval(t, `"1" == 1`, s); got != false {
		t.Errorf(`"1" == 1 = %#v, want false`, got)
	}
}

func TestEval_BooleanLogicAndTruthiness(t *testing.T) {
	s := mapScope{"yes": true, "no": false, "name": "x", "empty": "", "zero": int64(0), "list": []interface{}{}}
	cases := []struct {
		src  string
		want interface{}
	}{
		{`true && true`, true},
		{`true && false`, false},
		{`false || true`, true},
		{`yes && name`, "x"}, // && returns the operand (JS-style)
		{`no || name`, "x"},
		{`yes || name`, true},
		{`!yes`, false},
		{`!no`, true},
		{`!name`, false}, // truthy string
		{`!empty`, true},
		{`!zero`, true},
		{`!list`, true},
		{`name && no`, false},
		{`a || b || 5`, int64(5)},
	}
	for _, c := range cases {
		if got := mustEval(t, c.src, s); got != c.want {
			t.Errorf("%q = %#v, want %#v", c.src, got, c.want)
		}
	}
}

func TestEval_ShortCircuit(t *testing.T) {
	// RHS references a builtin that would error; must not be evaluated.
	s := mapScope{"ok": false}
	if got := mustEval(t, `ok && int("boom")`, s); got != false {
		t.Errorf("&& short-circuit failed: %#v", got)
	}
	s2 := mapScope{"ok": true}
	if got := mustEval(t, `ok || int("boom")`, s2); got != true {
		t.Errorf("|| short-circuit failed: %#v", got)
	}
	// when NOT short-circuited, the error surfaces
	if _, err := evalStr(t, `true && int("boom")`, s2); err == nil {
		t.Error("expected int(\"boom\") to error when reached")
	}
}

func TestEval_Precedence(t *testing.T) {
	s := mapScope{"a": false, "b": true, "c": true}
	// a || b && c  ==  a || (b && c)
	if got := mustEval(t, `a || b && c`, s); got != true {
		t.Errorf("a || b && c = %#v", got)
	}
	// !a == b  ==  (!a) == b
	if got := mustEval(t, `!a == b`, s); got != true {
		t.Errorf("!a == b = %#v", got)
	}
	// 1 + 2 == 3
	if got := mustEval(t, `1 + 2 == 3`, s); got != true {
		t.Errorf("1 + 2 == 3 = %#v", got)
	}
	// 2 > 1 && 3 > 2
	if got := mustEval(t, `2 > 1 && 3 > 2`, s); got != true {
		t.Errorf("chained comparison/logic = %#v", got)
	}
}

func TestEval_NullCoalesce(t *testing.T) {
	s := mapScope{"present": "v"}
	if got := mustEval(t, `missing ?? "fallback"`, s); got != "fallback" {
		t.Errorf("?? on missing = %#v", got)
	}
	if got := mustEval(t, `present ?? "fallback"`, s); got != "v" {
		t.Errorf("?? on present = %#v", got)
	}
	if got := mustEval(t, `missing ?? other ?? 3`, s); got != int64(3) {
		t.Errorf("chained ?? = %#v", got)
	}
}

func TestEval_PathResolution(t *testing.T) {
	s := mapScope{
		"result":        map[string]interface{}{"ok": true, "code": int64(200)},
		"result.ok":     true, // some scopes pre-flatten; both must work
		"items":         []interface{}{"a", "b", "c"},
		"cfg":           map[string]interface{}{"nested": map[string]interface{}{"v": int64(9)}},
		"<<x>>":         "should-not-be-used",
		"missing.thing": nil,
	}
	if got := mustEval(t, `result.ok`, s); got != true {
		t.Errorf("result.ok = %#v", got)
	}
	if got := mustEval(t, `items[1]`, s); got != "b" {
		t.Errorf("items[1] = %#v", got)
	}
	if got := mustEval(t, `items[9]`, s); got != nil {
		t.Errorf("out-of-range index = %#v, want nil", got)
	}
	if got := mustEval(t, `missing`, s); got != nil {
		t.Errorf("missing path = %#v, want nil", got)
	}
	// <<...>> token form
	sc := mapScope{"a.b": int64(7)}
	if got := mustEval(t, `<<a.b>> == 7`, sc); got != true {
		t.Errorf("<<a.b>> == 7 = %#v", got)
	}
}

func TestEval_Builtins(t *testing.T) {
	s := mapScope{"list": []interface{}{int64(1), int64(2)}, "s": "Hello"}
	cases := []struct {
		src  string
		want interface{}
	}{
		{`len("abc")`, int64(3)},
		{`len(list)`, int64(2)},
		{`len(missing)`, int64(0)},
		{`lower("AbC")`, "abc"},
		{`upper(s)`, "HELLO"},
		{`contains("hello", "ell")`, true},
		{`contains(list, 2)`, true},
		{`contains(list, 9)`, false},
		{`startsWith("hello", "he")`, true},
		{`endsWith("hello", "lo")`, true},
		{`int("42")`, int64(42)},
		{`int(3.9)`, int64(3)},
		{`float("1.5")`, 1.5},
		{`string(42)`, "42"},
		{`bool("")`, false},
		{`bool("x")`, true},
		{`default(null, 5)`, int64(5)},
		{`default("", "x")`, "x"},
		{`default("v", "x")`, "v"},
		{`has(s)`, true},
		{`has(missing)`, false},
	}
	for _, c := range cases {
		if got := mustEval(t, c.src, s); got != c.want {
			t.Errorf("%q = %#v, want %#v", c.src, got, c.want)
		}
	}
}

func TestEval_BuiltinErrors(t *testing.T) {
	s := mapScope{}
	for _, src := range []string{`int("nope")`, `float("x")`, `len(5)`, `unknownFn(1)`, `contains(5, 1)`} {
		if v, err := evalStr(t, src, s); err == nil {
			t.Errorf("%q: expected error, got %#v", src, v)
		}
	}
}

func TestEval_Truthy(t *testing.T) {
	falsy := []interface{}{nil, false, int64(0), float64(0), "", []interface{}{}, map[string]interface{}{}}
	for _, v := range falsy {
		if Truthy(v) {
			t.Errorf("Truthy(%#v) = true, want false", v)
		}
	}
	truthy := []interface{}{true, int64(1), float64(0.1), "x", []interface{}{1}, map[string]interface{}{"a": 1}, math.NaN()}
	for _, v := range truthy {
		if !Truthy(v) {
			t.Errorf("Truthy(%#v) = false, want true", v)
		}
	}
}

func TestEval_RealisticConditions(t *testing.T) {
	s := mapScope{
		"result": map[string]interface{}{"status": "completed", "code": int64(201)},
		"err":    map[string]interface{}{"retryable": true},
		"count":  int64(0),
	}
	cases := []struct {
		src  string
		want bool
	}{
		{`result.status == "completed" && result.code > 200`, true},
		{`result.status == "failed" || result.code == 201`, true},
		{`err.retryable == true`, true},
		{`count > 0`, false},
		{`count == 0`, true},
	}
	for _, c := range cases {
		got := Truthy(mustEval(t, c.src, s))
		if got != c.want {
			t.Errorf("%q => %v, want %v", c.src, got, c.want)
		}
	}
}
