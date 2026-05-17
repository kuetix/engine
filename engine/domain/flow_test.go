package domain

import (
	"testing"
)

func TestFlowTransition_OnSuccessWhen(t *testing.T) {
	t.Run("OnSuccessWhen field exists", func(t *testing.T) {
		conditionStr := "result > 0"
		transition := &FlowTransition{
			Name:          "test_transition",
			OnSuccessWhen: &conditionStr,
			To:            "next_state",
		}

		if transition.OnSuccessWhen == nil {
			t.Error("OnSuccessWhen should not be nil")
		}

		if *transition.OnSuccessWhen != conditionStr {
			t.Errorf("Expected OnSuccessWhen to be '%s', got '%s'", conditionStr, *transition.OnSuccessWhen)
		}
	})

	t.Run("OnSuccessWhen is optional", func(t *testing.T) {
		transition := &FlowTransition{
			Name: "test_transition",
			To:   "next_state",
		}

		if transition.OnSuccessWhen != nil {
			t.Error("OnSuccessWhen should be nil when not set")
		}
	})

	t.Run("Multiple transitions with different conditions", func(t *testing.T) {
		condition1 := "value == true"
		condition2 := "count > 5"

		transitions := []*FlowTransition{
			{
				Name:          "transition1",
				OnSuccessWhen: &condition1,
				To:            "state1",
			},
			{
				Name:          "transition2",
				OnSuccessWhen: &condition2,
				To:            "state2",
			},
			{
				Name: "transition3",
				To:   "state3",
			},
		}

		if *transitions[0].OnSuccessWhen != condition1 {
			t.Errorf("Expected first transition OnSuccessWhen to be '%s'", condition1)
		}

		if *transitions[1].OnSuccessWhen != condition2 {
			t.Errorf("Expected second transition OnSuccessWhen to be '%s'", condition2)
		}

		if transitions[2].OnSuccessWhen != nil {
			t.Error("Third transition should not have OnSuccessWhen set")
		}
	})
}

func TestFlowTransition_BackwardCompatibility(t *testing.T) {
	t.Run("Existing transitions work without OnSuccessWhen", func(t *testing.T) {
		ifCondition := "status == 'ready'"
		elseState := "error_state"

		transition := &FlowTransition{
			Name:  "legacy_transition",
			If:    &ifCondition,
			Else:  &elseState,
			To:    "success_state",
			True:  "true_state",
			False: "false_state",
		}

		// Verify all existing fields still work
		if transition.If == nil || *transition.If != ifCondition {
			t.Error("If condition should still work")
		}

		if transition.Else == nil || *transition.Else != elseState {
			t.Error("Else should still work")
		}

		if transition.True != "true_state" {
			t.Error("True transition should still work")
		}

		if transition.False != "false_state" {
			t.Error("False transition should still work")
		}

		// OnSuccessWhen should be nil for backward compatibility
		if transition.OnSuccessWhen != nil {
			t.Error("OnSuccessWhen should be nil for legacy transitions")
		}
	})
}

func TestFlowTransition_CombinedConditions(t *testing.T) {
	t.Run("Transition with both If and OnSuccessWhen", func(t *testing.T) {
		ifCond := "enabled == true"
		successCond := "result.status == 'completed'"

		transition := &FlowTransition{
			Name:          "combined_transition",
			If:            &ifCond,
			OnSuccessWhen: &successCond,
			To:            "next_state",
			True:          "success_state",
			False:         "failure_state",
		}

		if transition.If == nil || *transition.If != ifCond {
			t.Error("If condition should be set")
		}

		if transition.OnSuccessWhen == nil || *transition.OnSuccessWhen != successCond {
			t.Error("OnSuccessWhen condition should be set")
		}

		if transition.True != "success_state" {
			t.Error("True state should be set")
		}

		if transition.False != "failure_state" {
			t.Error("False state should be set")
		}
	})
}
