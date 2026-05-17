package workflow

import (
	"testing"

	"github.com/kuetix/engine/internal/wsl"
)

func TestWSLAttributes_Integration(t *testing.T) {
	t.Run("Parse if condition from WSL", func(t *testing.T) {
		// Simulate a WSL Graph with if condition
		graph := &wsl.Graph{
			WorkflowName: "test_workflow",
			Start:        "ConditionalState",
			Constants: map[string]interface{}{
				"enabled": "true",
			},
			Nodes: map[string]*wsl.Node{
				"ConditionalState": {
					Name:   "ConditionalState",
					Start:  true,
					IfExpr: &wsl.Expr{Raw: "$constants.enabled == true"},
					Action: &wsl.Action{
						Module: "services/common",
						Name:   "TestAction",
						As:     "result",
					},
					Edges: []wsl.Edge{
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							To:        "SuccessState",
						},
					},
				},
				"SuccessState": {
					Name:         "SuccessState",
					Terminal:     true,
					TerminalKind: "ok",
				},
			},
		}

		// Convert WSL Graph to schema
		schema := wslGraphToSchema(graph)

		if schema == nil {
			t.Fatal("Schema should not be nil")
		}

		// Check transitions
		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("Transitions should be a slice of maps")
		}

		// Find the ConditionalState transition
		var conditionalTransition map[string]interface{}
		for _, tr := range transitions {
			if name, ok := tr["name"].(string); ok && name == "ConditionalState" {
				conditionalTransition = tr
				break
			}
		}

		if conditionalTransition == nil {
			t.Fatal("ConditionalState transition not found")
		}

		// Verify if condition is set
		ifValue, ok := conditionalTransition["if"]
		if !ok {
			t.Error("if should be present in transition")
		}

		ifStr, ok := ifValue.(string)
		if !ok {
			t.Error("if should be a string")
		}

		expectedCondition := "$constants.enabled == true"
		if ifStr != expectedCondition {
			t.Errorf("Expected if to be '%s', got '%s'", expectedCondition, ifStr)
		}
	})

	t.Run("Parse continue_on_fail from WSL", func(t *testing.T) {
		graph := &wsl.Graph{
			WorkflowName: "test_workflow",
			Start:        "ContinueState",
			Nodes: map[string]*wsl.Node{
				"ContinueState": {
					Name:           "ContinueState",
					Start:          true,
					ContinueOnFail: true,
					Action: &wsl.Action{
						Module: "services/common",
						Name:   "MayFail",
					},
					Edges: []wsl.Edge{
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							To:        "NextState",
						},
					},
				},
				"NextState": {
					Name:         "NextState",
					Terminal:     true,
					TerminalKind: "ok",
				},
			},
		}

		schema := wslGraphToSchema(graph)
		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("Transitions should be a slice of maps")
		}

		var continueTransition map[string]interface{}
		for _, tr := range transitions {
			if name, ok := tr["name"].(string); ok && name == "ContinueState" {
				continueTransition = tr
				break
			}
		}

		if continueTransition == nil {
			t.Fatal("ContinueState transition not found")
		}

		// Verify continue_on_fail is set
		continueOnFail, ok := continueTransition["continue_on_fail"]
		if !ok {
			t.Error("continue_on_fail should be present")
		}

		continueOnFailBool, ok := continueOnFail.(bool)
		if !ok {
			t.Error("continue_on_fail should be a boolean")
		}

		if !continueOnFailBool {
			t.Error("continue_on_fail should be true")
		}
	})

	t.Run("Parse skipTo from WSL", func(t *testing.T) {
		graph := &wsl.Graph{
			WorkflowName: "test_workflow",
			Start:        "SkipState",
			Nodes: map[string]*wsl.Node{
				"SkipState": {
					Name:   "SkipState",
					Start:  true,
					SkipTo: true,
					Action: &wsl.Action{
						Module: "services/common",
						Name:   "Process",
					},
					Edges: []wsl.Edge{
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							To:        "FinalState",
						},
					},
				},
				"FinalState": {
					Name:         "FinalState",
					Terminal:     true,
					TerminalKind: "ok",
				},
			},
		}

		schema := wslGraphToSchema(graph)
		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("Transitions should be a slice of maps")
		}

		var skipTransition map[string]interface{}
		for _, tr := range transitions {
			if name, ok := tr["name"].(string); ok && name == "SkipState" {
				skipTransition = tr
				break
			}
		}

		if skipTransition == nil {
			t.Fatal("SkipState transition not found")
		}

		// Verify skipTo is set
		skipTo, ok := skipTransition["skipTo"]
		if !ok {
			t.Error("skipTo should be present")
		}

		skipToBool, ok := skipTo.(bool)
		if !ok {
			t.Error("skipTo should be a boolean")
		}

		if !skipToBool {
			t.Error("skipTo should be true")
		}
	})

	t.Run("Combined attributes", func(t *testing.T) {
		graph := &wsl.Graph{
			WorkflowName: "test_workflow",
			Start:        "CombinedState",
			Constants: map[string]interface{}{
				"threshold": "10",
			},
			Nodes: map[string]*wsl.Node{
				"CombinedState": {
					Name:           "CombinedState",
					Start:          true,
					IfExpr:         &wsl.Expr{Raw: "$result.valid == true"},
					ContinueOnFail: true,
					Action: &wsl.Action{
						Module: "services/common",
						Name:   "Process",
						As:     "processResult",
					},
					Edges: []wsl.Edge{
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							WhenExpr:  &wsl.Expr{Raw: "$processResult.score > $constants.threshold"},
							To:        "HighScore",
						},
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							To:        "DefaultPath",
						},
					},
				},
				"HighScore": {
					Name:         "HighScore",
					Terminal:     true,
					TerminalKind: "ok",
				},
				"DefaultPath": {
					Name:         "DefaultPath",
					Terminal:     true,
					TerminalKind: "ok",
				},
			},
		}

		schema := wslGraphToSchema(graph)
		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("Transitions should be a slice of maps")
		}

		var combinedTransition map[string]interface{}
		for _, tr := range transitions {
			if name, ok := tr["name"].(string); ok && name == "CombinedState" {
				combinedTransition = tr
				break
			}
		}

		if combinedTransition == nil {
			t.Fatal("CombinedState transition not found")
		}

		// Verify all attributes are set
		if ifExpr, ok := combinedTransition["if"].(string); !ok || ifExpr != "$result.valid == true" {
			t.Error("if condition should be set correctly")
		}

		if continueOnFail, ok := combinedTransition["continue_on_fail"].(bool); !ok || !continueOnFail {
			t.Error("continue_on_fail should be true")
		}

		if onSuccessWhen, ok := combinedTransition["on_success_when"].(string); !ok || onSuccessWhen != "$processResult.score > $constants.threshold" {
			t.Error("on_success_when should be set correctly")
		}
	})

	t.Run("Regular state without attributes", func(t *testing.T) {
		graph := &wsl.Graph{
			WorkflowName: "test_workflow",
			Start:        "SimpleState",
			Nodes: map[string]*wsl.Node{
				"SimpleState": {
					Name:  "SimpleState",
					Start: true,
					Action: &wsl.Action{
						Module: "services/common",
						Name:   "SimpleAction",
					},
					Edges: []wsl.Edge{
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							To:        "NextState",
						},
					},
				},
				"NextState": {
					Name:         "NextState",
					Terminal:     true,
					TerminalKind: "ok",
				},
			},
		}

		schema := wslGraphToSchema(graph)
		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("Transitions should be a slice of maps")
		}

		var simpleTransition map[string]interface{}
		for _, tr := range transitions {
			if name, ok := tr["name"].(string); ok && name == "SimpleState" {
				simpleTransition = tr
				break
			}
		}

		if simpleTransition == nil {
			t.Fatal("SimpleState transition not found")
		}

		// Verify attributes are NOT set for regular states
		if _, ok := simpleTransition["if"]; ok {
			t.Error("if should not be present for states without if condition")
		}

		if _, ok := simpleTransition["continue_on_fail"]; ok {
			t.Error("continue_on_fail should not be present for states without continue on fail")
		}

		if _, ok := simpleTransition["skipTo"]; ok {
			t.Error("skipTo should not be present for states without skip to")
		}

		if _, ok := simpleTransition["on_success_when"]; ok {
			t.Error("on_success_when should not be present for states without on success when")
		}
	})
}

func TestWSLAttributes_ParseValidation(t *testing.T) {
	t.Run("Parse WSL with if attribute", func(t *testing.T) {
		src := `module test_if
import services/common

workflow test {
  start: Check
  
  state Check {
    if $enabled == true
    action services/common/test.Check()
    on success -> Done
  }
  
  state Done {
    end ok
  }
}
`
		_, graphs, err := wsl.ParseAll(src, "")
		if err != nil {
			t.Fatalf("Failed to parse WSL: %v", err)
		}

		if len(graphs) == 0 {
			t.Fatal("No graphs generated")
		}

		graph := graphs["test"]
		if graph == nil {
			t.Fatal("test workflow not found")
		}

		checkNode := graph.Nodes["Check"]
		if checkNode == nil {
			t.Fatal("Check node not found")
		}

		if checkNode.IfExpr == nil {
			t.Error("IfExpr should be set")
		}

		if checkNode.IfExpr.Raw != "$enabled = = true" {
			t.Errorf("Expected if expression '$enabled = = true', got '%s'", checkNode.IfExpr.Raw)
		}
	})

	t.Run("Parse WSL with continue on fail", func(t *testing.T) {
		src := `module test_continue
import services/common

workflow test {
  start: MayFail
  
  state MayFail {
    continue on fail
    action services/common/test.MayFail()
    on success -> Done
  }
  
  state Done {
    end ok
  }
}
`
		_, graphs, err := wsl.ParseAll(src, "")
		if err != nil {
			t.Fatalf("Failed to parse WSL: %v", err)
		}

		graph := graphs["test"]
		if graph == nil {
			t.Fatal("test workflow not found")
		}

		mayFailNode := graph.Nodes["MayFail"]
		if mayFailNode == nil {
			t.Fatal("MayFail node not found")
		}

		if !mayFailNode.ContinueOnFail {
			t.Error("ContinueOnFail should be true")
		}
	})

	t.Run("Parse WSL with skip to", func(t *testing.T) {
		src := `module test_skip
import services/common

workflow test {
  start: Skip
  
  state Skip {
    skip to
    action services/common/test.Skip()
    on success -> Done
  }
  
  state Done {
    end ok
  }
}
`
		_, graphs, err := wsl.ParseAll(src, "")
		if err != nil {
			t.Fatalf("Failed to parse WSL: %v", err)
		}

		graph := graphs["test"]
		if graph == nil {
			t.Fatal("test workflow not found")
		}

		skipNode := graph.Nodes["Skip"]
		if skipNode == nil {
			t.Fatal("Skip node not found")
		}

		if !skipNode.SkipTo {
			t.Error("SkipTo should be true")
		}
	})

	t.Run("Parse WSL with end ok and on error transition", func(t *testing.T) {
		// Validates the fix for: state with 'end ok' and 'on error -> ErrorState' is valid.
		src := `module test_end_with_error
import services/common

workflow test {
  start: MainAction

  state MainAction {
    action services/common/test.MainAction()
    on error -> HandleError
    end ok
  }

  state HandleError {
    action services/common/test.HandleError()
    end fail
  }
}
`
		_, graphs, err := wsl.ParseAll(src, "")
		if err != nil {
			t.Fatalf("Expected no error for 'end ok' + 'on error' state, got: %v", err)
		}

		graph := graphs["test"]
		if graph == nil {
			t.Fatal("test workflow not found")
		}

		mainNode := graph.Nodes["MainAction"]
		if mainNode == nil {
			t.Fatal("MainAction node not found")
		}
		if !mainNode.Terminal {
			t.Error("MainAction should be terminal (end ok)")
		}
		if mainNode.TerminalKind != "ok" {
			t.Errorf("MainAction TerminalKind = %q, want 'ok'", mainNode.TerminalKind)
		}
		if len(mainNode.Edges) != 1 {
			t.Fatalf("MainAction edges = %d, want 1", len(mainNode.Edges))
		}
		if mainNode.Edges[0].Condition.Kind != wsl.CondError {
			t.Errorf("MainAction edge condition = %q, want %q", mainNode.Edges[0].Condition.Kind, wsl.CondError)
		}
		if mainNode.Edges[0].To != "HandleError" {
			t.Errorf("MainAction edge target = %q, want 'HandleError'", mainNode.Edges[0].To)
		}

		// Verify schema has correct final + false path
		schema := wslGraphToSchema(graph)
		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("transitions should be a slice of maps")
		}
		var mainTransition map[string]interface{}
		for _, tr := range transitions {
			if name, ok := tr["name"].(string); ok && name == "MainAction" {
				mainTransition = tr
				break
			}
		}
		if mainTransition == nil {
			t.Fatal("MainAction transition not found in schema")
		}
		if mainTransition["type"] != "final" {
			t.Errorf("MainAction transition type = %v, want 'final'", mainTransition["type"])
		}
		if mainTransition["final_kind"] != "ok" {
			t.Errorf("MainAction transition final_kind = %v, want 'ok'", mainTransition["final_kind"])
		}
		// Error path should be in 'false' key
		if mainTransition["false"] == nil {
			t.Error("MainAction transition should have a 'false' (error) path")
		}
	})
}
