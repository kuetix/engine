package workflow

import (
	"strings"
	"testing"

	"github.com/kuetix/engine/engine/domain"
)

func TestCorrectFlow_ResolvesUnderscoreBranchTargetsToNextTransition(t *testing.T) {
	e := &Engine{}
	elseTarget := "_"
	flow := &domain.Flow{
		Transitions: []*domain.FlowTransition{
			{
				Name:  "first",
				To:    "state_one",
				From:  []string{"_"},
				True:  "_",
				False: "_",
				Else:  &elseTarget,
			},
			{
				Name: "second",
				To:   "state_two",
			},
		},
		States: []*domain.FlowState{},
	}

	if err := e.CorrectFlow(flow); err != nil {
		t.Fatalf("CorrectFlow failed: %v", err)
	}

	first := flow.Transitions[0]
	if first.True != "state_two#1" {
		t.Fatalf("expected true target to resolve to next state 'state_two#1', got: %q", first.True)
	}
	if first.False != "state_two#1" {
		t.Fatalf("expected false target to resolve to next state 'state_two#1', got: %q", first.False)
	}
	if first.Else == nil || *first.Else != "state_two#1" {
		t.Fatalf("expected else target to resolve to next state 'state_two#1', got: %v", first.Else)
	}
}

func TestCorrectFlow_TrailingTrueUnderscoreMakesTransitionFinal(t *testing.T) {
	e := &Engine{}
	flow := &domain.Flow{
		Transitions: []*domain.FlowTransition{
			{
				Name: "last",
				To:   "state_one",
				From: []string{"_"},
				True: "_",
			},
		},
		States: []*domain.FlowState{},
	}

	if err := e.CorrectFlow(flow); err != nil {
		t.Fatalf("expected trailing true '_' to be accepted as final, got error: %v", err)
	}
	last := flow.Transitions[0]
	if last.True != "" {
		t.Fatalf("expected trailing true '_' to clear true target, got: %q", last.True)
	}
	if last.Type != domain.StateFinal {
		t.Fatalf("expected transition type final, got: %q", last.Type)
	}
	if last.FinalKind != "ok" {
		t.Fatalf("expected transition final_kind ok, got: %q", last.FinalKind)
	}
}

func TestCorrectFlow_RejectsTrailingNonSuccessUnderscore(t *testing.T) {
	e := &Engine{}
	elseTarget := "_"
	flow := &domain.Flow{
		Transitions: []*domain.FlowTransition{
			{
				Name:  "last",
				To:    "state_one",
				From:  []string{"_"},
				False: "_",
				Else:  &elseTarget,
			},
		},
		States: []*domain.FlowState{},
	}

	err := e.CorrectFlow(flow)
	if err == nil {
		t.Fatal("expected error for trailing non-success '_'")
	}
	if !strings.Contains(err.Error(), "'_' in false branch requires a following transition or explicit target") {
		t.Fatalf("unexpected error: %v", err)
	}
}
