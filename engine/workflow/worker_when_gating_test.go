package workflow

import (
	"testing"

	"github.com/kuetix/engine/engine/domain"
)

// newGatingSession builds a WorkerSessionContext whose context holds the given
// values, for testing `when` / `if` condition evaluation end to end
// (parse -> resolve against context -> evaluate -> truthiness).
func newGatingSession(values map[string]interface{}) (*WorkerSessionContext, *guardWorker) {
	gw := newGuardWorker(func(int) string { return "" })
	for k, v := range values {
		gw.ctx.m[k] = v
	}
	wsc := &WorkerSessionContext{
		WorkflowContext: gw.ctx,
		Worker:          gw,
		Flow:            &domain.Flow{CurrentTransition: &domain.FlowTransition{}},
		Parser:          NewParser(),
	}
	return wsc, gw
}

func TestWhenGating_EvaluatesConditions(t *testing.T) {
	wsc, _ := newGatingSession(map[string]interface{}{
		"result": map[string]interface{}{"ok": true, "code": int64(201), "status": "completed"},
		"err":    map[string]interface{}{"retryable": true},
		"count":  int64(0),
		"flag":   false,
	})

	cases := []struct {
		cond string
		want bool
	}{
		{`result.ok == true`, true},
		{`result.ok == false`, false}, // the case that never gated before
		{`result.code > 200`, true},
		{`result.code >= 500`, false},
		{`result.status == "completed"`, true},
		{`result.status == "failed"`, false},
		{`result.status == "completed" && result.code > 200`, true},
		{`result.status == "completed" && result.code > 999`, false},
		{`count == 0`, true},
		{`count > 0`, false},
		{`err.retryable == true`, true},
		{`flag`, false},
		{`!flag`, true},
		{`result.ok`, true},
		{`missing.path == "x"`, false},
	}
	for _, c := range cases {
		pass := evaluateTransitionCondition(c.cond, wsc)
		if pass == nil {
			t.Fatalf("%q: unrecoverable error", c.cond)
		}
		if *pass != c.want {
			t.Errorf("%q: gate = %v, want %v", c.cond, *pass, c.want)
		}
	}
}

func TestWhenGating_EmptyConditionPasses(t *testing.T) {
	wsc, _ := newGatingSession(nil)
	if p := evaluateTransitionCondition("", wsc); p == nil || !*p {
		t.Fatalf("empty condition should pass, got %v", p)
	}
}

func TestWhenGating_LegacyTemplateFallback(t *testing.T) {
	// A string that is not a parseable expression falls back to the legacy
	// ParseTemplate path (only the literal "false" fails).
	wsc, _ := newGatingSession(nil)
	if p := evaluateTransitionCondition("true", wsc); p == nil || !*p {
		t.Fatalf(`"true" should pass, got %v`, p)
	}
}

func TestWhenGating_EvalErrorSurfacesAndFailsClosed(t *testing.T) {
	// Parseable expression, but a runtime type error (ordering a string vs int).
	wsc, gw := newGatingSession(map[string]interface{}{"name": "abc"})
	pass := evaluateTransitionCondition(`name < 5`, wsc)
	if pass == nil {
		return // treated as unrecoverable; also acceptable
	}
	if *pass {
		t.Error("a condition that errored at eval time should not pass")
	}
	if gw.GetError() == nil {
		t.Error("eval error should be surfaced on the worker")
	}
}

func TestWhenGating_TokenSyntaxStillWorks(t *testing.T) {
	wsc, _ := newGatingSession(map[string]interface{}{
		"parent": map[string]interface{}{"result": "success"},
	})
	pass := evaluateTransitionCondition(`<<parent.result>> == "success"`, wsc)
	if pass == nil || !*pass {
		t.Fatalf("<<token>> comparison should pass, got %v", pass)
	}
}
