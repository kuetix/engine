package wsl

import (
	"strings"
	"testing"
)

// TestSWSLParallel_HeadOfChain verifies `action()[count: N] as Alias -> next -> .`
// lowers to a parallel-fork state joined by a synthetic wait state, with the
// chain continuation wired onto the join (so it only runs after all branches
// finish).
func TestSWSLParallel_HeadOfChain(t *testing.T) {
	src := `
module startup

commands.Register()[count: 6] as Reg -> commands.Index.Prepare() -> .
`
	mod, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL error: %v", err)
	}
	if len(mod.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(mod.Workflows))
	}
	wf := mod.Workflows[0]

	fork, ok := wf.States["Reg"]
	if !ok {
		t.Fatal("fork state 'Reg' not found")
	}
	if !fork.Parallel || fork.ParallelCount != 6 {
		t.Errorf("fork state: parallel=%v count=%d, want true/6", fork.Parallel, fork.ParallelCount)
	}
	if fork.Action == nil || fork.Action.Name != "Register" || fork.Action.As != "Reg" {
		t.Errorf("fork action not wired correctly: %+v", fork.Action)
	}
	if wf.Start != "Reg" {
		t.Errorf("workflow start: got %q, want Reg", wf.Start)
	}
	if len(fork.Transitions) != 1 || fork.Transitions[0].Target != "Reg_join" {
		t.Fatalf("fork transitions: got %+v, want single edge to Reg_join", fork.Transitions)
	}

	join, ok := wf.States["Reg_join"]
	if !ok {
		t.Fatal("join state 'Reg_join' not found")
	}
	if !join.Wait || join.JoinTarget != "Reg" {
		t.Errorf("join state: wait=%v joinTarget=%q, want true/Reg", join.Wait, join.JoinTarget)
	}
	if len(join.Transitions) != 1 {
		t.Fatalf("join transitions: got %d, want 1 (continuation to next action)", len(join.Transitions))
	}
	if join.Transitions[0].Condition.Kind != CondSuccess {
		t.Errorf("join continuation should be on success, got %v", join.Transitions[0].Condition.Kind)
	}

	nextState, ok := wf.States[join.Transitions[0].Target]
	if !ok {
		t.Fatalf("join's continuation target %q not found", join.Transitions[0].Target)
	}
	if nextState.Action == nil || nextState.Action.Name != "Prepare" {
		t.Errorf("continuation action not wired: %+v", nextState.Action)
	}
	if nextState.End == nil || nextState.End.Kind != "ok" {
		t.Errorf("chain terminal not wired on continuation state: %+v", nextState.End)
	}
}

// TestSWSLParallel_ErrorBindingOnJoin verifies that an error binding on a
// forking action attaches "on error" to the join state, matching the
// semantics of `wait { join X; on error -> ... }` in full WSL.
func TestSWSLParallel_ErrorBindingOnJoin(t *testing.T) {
	src := `
module startup

commands.Register()[count: 3] as Reg <- errors.OnAnyError() -> .
`
	mod, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL error: %v", err)
	}
	wf := mod.Workflows[0]

	join, ok := wf.States["Reg_join"]
	if !ok {
		t.Fatal("join state 'Reg_join' not found")
	}
	if join.End == nil || join.End.Kind != "ok" {
		t.Errorf("join terminal (success path): got %+v, want end ok", join.End)
	}

	var hasErrorTransition bool
	for _, tr := range join.Transitions {
		if tr.Condition.Kind == CondError {
			hasErrorTransition = true
			target, ok := wf.States[tr.Target]
			if !ok {
				t.Fatalf("error transition target %q not found", tr.Target)
			}
			if target.Action == nil || target.Action.Name != "OnAnyError" {
				t.Errorf("error handler state action not wired: %+v", target.Action)
			}
		}
	}
	if !hasErrorTransition {
		t.Error("join state should have an 'on error' transition from the error binding")
	}
}

// TestSWSLParallel_MidChain verifies a fork/join pair can appear after a
// preceding action in the chain, not just as the head.
func TestSWSLParallel_MidChain(t *testing.T) {
	src := `
module startup

commands.Setup() -> commands.Register()[count: 4] as Reg -> commands.Finish() -> .
`
	mod, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL error: %v", err)
	}
	wf := mod.Workflows[0]

	setup, ok := wf.States["commands.Setup"]
	if !ok {
		t.Fatal("commands.Setup state not found")
	}
	if len(setup.Transitions) != 1 || setup.Transitions[0].Target != "Reg" {
		t.Fatalf("Setup should transition to fork state 'Reg', got %+v", setup.Transitions)
	}

	fork, ok := wf.States["Reg"]
	if !ok || !fork.Parallel {
		t.Fatalf("fork state 'Reg' not found or not parallel: %+v", fork)
	}

	join, ok := wf.States["Reg_join"]
	if !ok {
		t.Fatal("join state 'Reg_join' not found")
	}
	if len(join.Transitions) != 1 {
		t.Fatalf("join transitions: got %d, want 1", len(join.Transitions))
	}
	finishState, ok := wf.States[join.Transitions[0].Target]
	if !ok || finishState.Action == nil || finishState.Action.Name != "Finish" {
		t.Errorf("join should continue to Finish state, got %+v", finishState)
	}
}

// TestSWSLParallel_MissingCount verifies the required 'count' attribute is
// still enforced in SWSL, matching the full-WSL validation.
func TestSWSLParallel_MissingCount(t *testing.T) {
	src := `
module startup

commands.Register()[shard: 1] as Reg -> .
`
	_, err := ParseSimplifiedWSL(src)
	if err == nil {
		t.Fatal("expected error for missing 'count' attribute")
	}
	if !strings.Contains(err.Error(), "'count' attribute") {
		t.Errorf("error %q does not mention missing count attribute", err.Error())
	}
}

// TestSWSLParallel_SchemaEndToEnd verifies the SWSL fork/join pair survives
// the full pipeline through to the engine-facing schema, matching the
// full-WSL end-to-end coverage.
func TestSWSLParallel_SchemaEndToEnd(t *testing.T) {
	src := `
module startup

commands.Register()[count: 6] as Reg -> commands.Finish() -> .
`
	_, graphs, err := ParseAllSimplified(src, "startup")
	if err != nil {
		t.Fatalf("ParseAllSimplified: %v", err)
	}
	g, ok := graphs["main"]
	if !ok {
		t.Fatalf("graph 'main' not found, got: %v", graphs)
	}
	forkNode, ok := g.Nodes["Reg"]
	if !ok || !forkNode.Parallel || forkNode.ParallelCount != 6 {
		t.Fatalf("fork node missing or malformed: %+v", forkNode)
	}
	joinNode, ok := g.Nodes["Reg_join"]
	if !ok || !joinNode.Wait || joinNode.JoinTarget != "Reg" {
		t.Fatalf("join node missing or malformed: %+v", joinNode)
	}
}
