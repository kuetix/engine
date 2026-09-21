package workflow

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/kuetix/engine/internal/wsl"
)

// Expression evaluation.
//
// A parsed wsl.ExprNode is evaluated against a Scope that resolves context
// paths (result aliases, constants, branch info, …). Values are normalised Go
// types: nil, bool, int64, float64, string, []interface{}, map[string]interface{}.
//
// Semantics follow ENGINE_EVOLUTION_PLAN.md Phase 1 with decision #3
// (truthiness allowed): `&&` / `||` are JS-style and return an operand;
// `when` / `if` apply Truthy() to the final result.

// ExprScope resolves a context path to a value. found is false when the path is
// not present in scope (distinct from a present nil).
type ExprScope interface {
	ResolvePath(path string) (value interface{}, found bool)
}

// scopeFunc adapts a function to ExprScope.
type scopeFunc func(path string) (interface{}, bool)

func (f scopeFunc) ResolvePath(path string) (interface{}, bool) { return f(path) }

// EvalExpr evaluates a parsed expression tree.
func EvalExpr(node wsl.ExprNode, scope ExprScope) (interface{}, error) {
	if node == nil {
		return nil, fmt.Errorf("nil expression")
	}
	switch n := node.(type) {
	case *wsl.LitExpr:
		switch n.Kind {
		case "int":
			return n.Int, nil
		case "float":
			return n.Float, nil
		case "string":
			return n.Str, nil
		case "bool":
			return n.Bool, nil
		case "null":
			return nil, nil
		}
		return nil, fmt.Errorf("unknown literal kind %q", n.Kind)

	case *wsl.PathExpr:
		v, _ := resolvePathValue(scope, n.Path)
		return normalizeValue(v), nil

	case *wsl.InterpExpr:
		var sb strings.Builder
		for _, part := range n.Parts {
			v, err := EvalExpr(part, scope)
			if err != nil {
				return nil, err
			}
			sb.WriteString(stringifyValue(v))
		}
		return sb.String(), nil

	case *wsl.ListExpr:
		out := make([]interface{}, 0, len(n.Elems))
		for _, e := range n.Elems {
			v, err := EvalExpr(e, scope)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil

	case *wsl.MapExpr:
		out := make(map[string]interface{}, len(n.Keys))
		for i, k := range n.Keys {
			v, err := EvalExpr(n.Vals[i], scope)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil

	case *wsl.IndexExpr:
		base, err := EvalExpr(n.X, scope)
		if err != nil {
			return nil, err
		}
		idx, err := EvalExpr(n.Index, scope)
		if err != nil {
			return nil, err
		}
		return evalIndex(base, idx)

	case *wsl.UnaryExpr:
		v, err := EvalExpr(n.X, scope)
		if err != nil {
			return nil, err
		}
		switch n.Op {
		case "!":
			return !Truthy(v), nil
		case "-":
			switch x := toNumber(v).(type) {
			case int64:
				return -x, nil
			case float64:
				return -x, nil
			}
			return nil, fmt.Errorf("unary '-' needs a number, got %s", typeName(v))
		}
		return nil, fmt.Errorf("unknown unary operator %q", n.Op)

	case *wsl.CallExpr:
		return evalCall(n, scope)

	case *wsl.BinaryExpr:
		return evalBinary(n, scope)
	}
	return nil, fmt.Errorf("unsupported expression node %T", node)
}

func evalBinary(n *wsl.BinaryExpr, scope ExprScope) (interface{}, error) {
	// short-circuiting operators evaluate the right side lazily
	switch n.Op {
	case "&&":
		l, err := EvalExpr(n.L, scope)
		if err != nil {
			return nil, err
		}
		if !Truthy(l) {
			return l, nil
		}
		return EvalExpr(n.R, scope)
	case "||":
		l, err := EvalExpr(n.L, scope)
		if err != nil {
			return nil, err
		}
		if Truthy(l) {
			return l, nil
		}
		return EvalExpr(n.R, scope)
	case "??":
		l, err := EvalExpr(n.L, scope)
		if err != nil {
			return nil, err
		}
		if l != nil {
			return l, nil
		}
		return EvalExpr(n.R, scope)
	}

	l, err := EvalExpr(n.L, scope)
	if err != nil {
		return nil, err
	}
	r, err := EvalExpr(n.R, scope)
	if err != nil {
		return nil, err
	}

	switch n.Op {
	case "==":
		return valuesEqual(l, r), nil
	case "!=":
		return !valuesEqual(l, r), nil
	case "<", "<=", ">", ">=":
		return compare(n.Op, l, r)
	case "+":
		// string concatenation when either side is a string
		if ls, ok := l.(string); ok {
			if rs, ok := r.(string); ok {
				return ls + rs, nil
			}
			return nil, fmt.Errorf("'+': cannot add %s to string (use \"${...}\" interpolation)", typeName(r))
		}
		if _, ok := r.(string); ok {
			return nil, fmt.Errorf("'+': cannot add string to %s", typeName(l))
		}
		return arith("+", l, r)
	case "-", "*", "/", "%":
		return arith(n.Op, l, r)
	}
	return nil, fmt.Errorf("unknown operator %q", n.Op)
}

func evalCall(n *wsl.CallExpr, scope ExprScope) (interface{}, error) {
	// has(path) inspects presence, so it needs the path, not its value.
	if n.Name == "has" {
		if len(n.Args) != 1 {
			return nil, fmt.Errorf("has() takes exactly 1 argument")
		}
		if pe, ok := n.Args[0].(*wsl.PathExpr); ok {
			v, found := resolvePathValue(scope, pe.Path)
			return found && v != nil, nil
		}
		v, err := EvalExpr(n.Args[0], scope)
		if err != nil {
			return nil, err
		}
		return v != nil, nil
	}

	args := make([]interface{}, len(n.Args))
	for i, a := range n.Args {
		v, err := EvalExpr(a, scope)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}

	switch n.Name {
	case "len":
		if len(args) != 1 {
			return nil, fmt.Errorf("len() takes exactly 1 argument")
		}
		switch v := args[0].(type) {
		case string:
			return int64(len(v)), nil
		case []interface{}:
			return int64(len(v)), nil
		case map[string]interface{}:
			return int64(len(v)), nil
		case nil:
			return int64(0), nil
		}
		return nil, fmt.Errorf("len(): %s has no length", typeName(args[0]))
	case "lower", "upper":
		if len(args) != 1 {
			return nil, fmt.Errorf("%s() takes exactly 1 argument", n.Name)
		}
		s := stringifyValue(args[0])
		if n.Name == "lower" {
			return strings.ToLower(s), nil
		}
		return strings.ToUpper(s), nil
	case "contains":
		if len(args) != 2 {
			return nil, fmt.Errorf("contains() takes 2 arguments")
		}
		switch hay := args[0].(type) {
		case string:
			return strings.Contains(hay, stringifyValue(args[1])), nil
		case []interface{}:
			for _, e := range hay {
				if valuesEqual(e, args[1]) {
					return true, nil
				}
			}
			return false, nil
		case map[string]interface{}:
			_, ok := hay[stringifyValue(args[1])]
			return ok, nil
		case nil:
			return false, nil
		}
		return nil, fmt.Errorf("contains(): %s is not searchable", typeName(args[0]))
	case "startsWith", "endsWith":
		if len(args) != 2 {
			return nil, fmt.Errorf("%s() takes 2 arguments", n.Name)
		}
		s := stringifyValue(args[0])
		sub := stringifyValue(args[1])
		if n.Name == "startsWith" {
			return strings.HasPrefix(s, sub), nil
		}
		return strings.HasSuffix(s, sub), nil
	case "int":
		if len(args) != 1 {
			return nil, fmt.Errorf("int() takes exactly 1 argument")
		}
		return toInt(args[0])
	case "float":
		if len(args) != 1 {
			return nil, fmt.Errorf("float() takes exactly 1 argument")
		}
		return toFloat(args[0])
	case "string":
		if len(args) != 1 {
			return nil, fmt.Errorf("string() takes exactly 1 argument")
		}
		return stringifyValue(args[0]), nil
	case "bool":
		if len(args) != 1 {
			return nil, fmt.Errorf("bool() takes exactly 1 argument")
		}
		return Truthy(args[0]), nil
	case "default":
		if len(args) != 2 {
			return nil, fmt.Errorf("default() takes 2 arguments")
		}
		if args[0] == nil || args[0] == "" {
			return args[1], nil
		}
		return args[0], nil
	}
	return nil, fmt.Errorf("unknown builtin %q", n.Name)
}

// resolvePathValue resolves a dotted path against the scope. It first asks the
// scope for the whole path (so a scope that does its own dotted resolution, or
// one with pre-flattened keys, wins). Failing that it resolves the longest
// known prefix and walks the remaining segments as map/list access.
func resolvePathValue(scope ExprScope, path string) (interface{}, bool) {
	if v, ok := scope.ResolvePath(path); ok {
		return v, true
	}
	segs := strings.Split(path, ".")
	if len(segs) == 1 {
		return nil, false
	}
	for cut := len(segs) - 1; cut >= 1; cut-- {
		prefix := strings.Join(segs[:cut], ".")
		base, ok := scope.ResolvePath(prefix)
		if !ok {
			continue
		}
		cur := normalizeValue(base)
		okAll := true
		for _, seg := range segs[cut:] {
			next, stepOk := stepInto(cur, seg)
			if !stepOk {
				okAll = false
				break
			}
			cur = normalizeValue(next)
		}
		if okAll {
			return cur, true
		}
	}
	return nil, false
}

func stepInto(v interface{}, key string) (interface{}, bool) {
	switch x := normalizeValue(v).(type) {
	case map[string]interface{}:
		val, ok := x[key]
		return val, ok
	case []interface{}:
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || i >= len(x) {
			return nil, false
		}
		return x[i], true
	}
	return nil, false
}

// --- value helpers ---

// Truthy implements the falsy set from decision #3: nil, false, 0, 0.0, "",
// empty list, empty map. Everything else is truthy.
func Truthy(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	case []interface{}:
		return len(x) > 0
	case map[string]interface{}:
		return len(x) > 0
	}
	if nv := normalizeValue(v); !sameConcreteType(nv, v) {
		return Truthy(nv)
	}
	// unknown non-nil concrete type (e.g. a struct): treat presence as truthy
	return v != nil
}

func sameConcreteType(a, b interface{}) bool {
	return fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b)
}

// normalizeValue coerces the many concrete numeric/collection types that flow
// through the workflow context into the evaluator's canonical set.
func normalizeValue(v interface{}) interface{} {
	switch x := v.(type) {
	case nil, bool, string, int64, float64, []interface{}, map[string]interface{}:
		return v
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case uint:
		return int64(x)
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return int64(x)
	case float32:
		return float64(x)
	case []string:
		out := make([]interface{}, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, val := range x {
			out[fmt.Sprintf("%v", k)] = val
		}
		return out
	}
	return v
}

func typeName(v interface{}) string {
	switch normalizeValue(v).(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case int64:
		return "int"
	case float64:
		return "float"
	case string:
		return "string"
	case []interface{}:
		return "list"
	case map[string]interface{}:
		return "map"
	}
	return fmt.Sprintf("%T", v)
}

func stringifyValue(v interface{}) string {
	switch x := normalizeValue(v).(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", v)
}

func toNumber(v interface{}) interface{} {
	switch x := normalizeValue(v).(type) {
	case int64:
		return x
	case float64:
		return x
	case string:
		if iv, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
			return iv
		}
		if fv, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return fv
		}
	}
	return nil
}

func toInt(v interface{}) (interface{}, error) {
	switch n := toNumber(v).(type) {
	case int64:
		return n, nil
	case float64:
		return int64(n), nil
	}
	return nil, fmt.Errorf("int(): cannot convert %s %q", typeName(v), stringifyValue(v))
}

func toFloat(v interface{}) (interface{}, error) {
	switch n := toNumber(v).(type) {
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	}
	return nil, fmt.Errorf("float(): cannot convert %s %q", typeName(v), stringifyValue(v))
}

func arith(op string, l, r interface{}) (interface{}, error) {
	ln, rn := toNumber(l), toNumber(r)
	if ln == nil {
		return nil, fmt.Errorf("'%s': %s is not a number", op, typeName(l))
	}
	if rn == nil {
		return nil, fmt.Errorf("'%s': %s is not a number", op, typeName(r))
	}
	li, lIsInt := ln.(int64)
	ri, rIsInt := rn.(int64)
	if lIsInt && rIsInt {
		switch op {
		case "+":
			return li + ri, nil
		case "-":
			return li - ri, nil
		case "*":
			return li * ri, nil
		case "/":
			if ri == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			if li%ri == 0 {
				return li / ri, nil
			}
			return float64(li) / float64(ri), nil
		case "%":
			if ri == 0 {
				return nil, fmt.Errorf("modulo by zero")
			}
			return li % ri, nil
		}
	}
	lf := asFloat(ln)
	rf := asFloat(rn)
	switch op {
	case "+":
		return lf + rf, nil
	case "-":
		return lf - rf, nil
	case "*":
		return lf * rf, nil
	case "/":
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return lf / rf, nil
	case "%":
		if rf == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return math.Mod(lf, rf), nil
	}
	return nil, fmt.Errorf("unknown arithmetic operator %q", op)
}

func asFloat(n interface{}) float64 {
	switch x := n.(type) {
	case int64:
		return float64(x)
	case float64:
		return x
	}
	return 0
}

func valuesEqual(l, r interface{}) bool {
	l = normalizeValue(l)
	r = normalizeValue(r)
	if l == nil || r == nil {
		return l == nil && r == nil
	}
	// numeric cross-type
	ln, lNum := numericValue(l)
	rn, rNum := numericValue(r)
	if lNum && rNum {
		return ln == rn
	}
	switch lv := l.(type) {
	case string:
		rv, ok := r.(string)
		return ok && lv == rv
	case bool:
		rv, ok := r.(bool)
		return ok && lv == rv
	case []interface{}:
		rv, ok := r.([]interface{})
		if !ok || len(lv) != len(rv) {
			return false
		}
		for i := range lv {
			if !valuesEqual(lv[i], rv[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		rv, ok := r.(map[string]interface{})
		if !ok || len(lv) != len(rv) {
			return false
		}
		for k := range lv {
			if !valuesEqual(lv[k], rv[k]) {
				return false
			}
		}
		return true
	}
	return false
}

func numericValue(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func compare(op string, l, r interface{}) (interface{}, error) {
	ln, lNum := numericValue(normalizeValue(l))
	rn, rNum := numericValue(normalizeValue(r))
	if lNum && rNum {
		switch op {
		case "<":
			return ln < rn, nil
		case "<=":
			return ln <= rn, nil
		case ">":
			return ln > rn, nil
		case ">=":
			return ln >= rn, nil
		}
	}
	ls, lStr := normalizeValue(l).(string)
	rs, rStr := normalizeValue(r).(string)
	if lStr && rStr {
		switch op {
		case "<":
			return ls < rs, nil
		case "<=":
			return ls <= rs, nil
		case ">":
			return ls > rs, nil
		case ">=":
			return ls >= rs, nil
		}
	}
	return nil, fmt.Errorf("'%s': cannot order %s and %s", op, typeName(l), typeName(r))
}

func evalIndex(base, idx interface{}) (interface{}, error) {
	switch b := normalizeValue(base).(type) {
	case []interface{}:
		n, ok := numericValue(normalizeValue(idx))
		if !ok {
			return nil, fmt.Errorf("list index must be a number, got %s", typeName(idx))
		}
		i := int(n)
		if i < 0 || i >= len(b) {
			return nil, nil
		}
		return b[i], nil
	case map[string]interface{}:
		key := stringifyValue(idx)
		return b[key], nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("cannot index %s", typeName(base))
}

// --- integration with the WSL parser for cached parse trees ---

var exprParseCache sync.Map // map[string]exprParseResult

type exprParseResult struct {
	node wsl.ExprNode
	err  error
}

// parseConditionExpr parses (and caches) a raw condition string into a tree.
// Safe for concurrent use (parallel branches evaluate conditions concurrently).
func parseConditionExpr(raw string) (wsl.ExprNode, error) {
	if r, ok := exprParseCache.Load(raw); ok {
		res := r.(exprParseResult)
		return res.node, res.err
	}
	node, err := wsl.ParseExpr(raw)
	exprParseCache.Store(raw, exprParseResult{node, err})
	return node, err
}

// sessionScope resolves expression paths against the running workflow context,
// reusing the engine's existing path resolver (dotted paths, constants, options,
// `$`-prefixed variables, `|int` coercions).
type sessionScope struct{ wsc *WorkerSessionContext }

func (s sessionScope) ResolvePath(path string) (interface{}, bool) {
	candidates := []string{path}
	if strings.HasPrefix(path, "$") {
		candidates = append(candidates, strings.TrimPrefix(path, "$"))
	} else {
		candidates = append(candidates, "$"+path)
	}
	for _, c := range candidates {
		_, v, err := s.wsc.GetProperty(c)
		if err != nil || v == nil {
			continue
		}
		// GetProperty echoes the key back when the path is not found.
		if sv, ok := v.(string); ok && (sv == c || sv == path) {
			continue
		}
		return normalizeValue(v), true
	}
	return nil, false
}

// evaluateTransitionCondition resolves a `when` / `if` condition to a pass/fail
// decision. It first tries the expression evaluator; if the string is not a
// parseable expression it falls back to the legacy template behaviour
// (ParseTemplate, "false" means fail). A nil return means an unrecoverable
// error was raised on the worker and the caller should abort the state.
func evaluateTransitionCondition(raw string, wsc *WorkerSessionContext) *bool {
	res, handled, eerr := evalWorkflowCondition(raw, wsc)
	if handled {
		if eerr != nil {
			if !wsc.Worker.HandleError(eerr, http.StatusInternalServerError) {
				return nil
			}
			// error surfaced but recoverable: treat the guard as failing
			f := false
			return &f
		}
		return &res
	}
	// legacy fallback: token substitution, only literal "false" fails
	condition, err := wsc.Parser.ParseTemplate(raw)
	if !wsc.Worker.HandleError(err, http.StatusInternalServerError) {
		return nil
	}
	pass := condition != "false"
	return &pass
}

// applyLetBindings evaluates the current transition's `let` bindings in order
// and writes each result into the session context, so later bindings, the
// `if` guard, action arguments and `when` guards can reference them.
func applyLetBindings(wsc *WorkerSessionContext) error {
	if wsc.Flow == nil || wsc.Flow.CurrentTransition == nil {
		return nil
	}
	for _, lb := range wsc.Flow.CurrentTransition.Lets {
		node, err := parseConditionExpr(lb.Expr)
		if err != nil {
			return fmt.Errorf("let %s: %w", lb.Name, err)
		}
		v, err := EvalExpr(node, sessionScope{wsc})
		if err != nil {
			return fmt.Errorf("let %s: %w", lb.Name, err)
		}
		wsc.SetValue(lb.Name, v)
	}
	return nil
}

// evalWorkflowCondition evaluates a `when` / `if` condition string against the
// session. handled is false when the string is not a parseable expression (the
// caller should fall back to legacy template behaviour). result is the
// truthiness of the evaluated expression.
func evalWorkflowCondition(raw string, wsc *WorkerSessionContext) (result bool, handled bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true, true, nil
	}
	node, perr := parseConditionExpr(raw)
	if perr != nil {
		return false, false, perr
	}
	v, eerr := EvalExpr(node, sessionScope{wsc})
	if eerr != nil {
		return false, true, eerr
	}
	return Truthy(v), true, nil
}
