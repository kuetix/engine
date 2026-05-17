package workflow

import (
	"testing"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/internal/wsl"
)

func TestWSLOnSuccessWhen_Integration(t *testing.T) {
	t.Run("Parse on success when from WSL", func(t *testing.T) {
		// Simulate a WSL Graph with on success when condition
		graph := &wsl.Graph{
			WorkflowName: "test_workflow",
			Start:        "StartState",
			Constants: map[string]interface{}{
				"version": "1.0.0",
			},
			Nodes: map[string]*wsl.Node{
				"StartState": {
					Name:  "StartState",
					Start: true,
					Action: &wsl.Action{
						Module: "services/common",
						Name:   "TestAction",
						As:     "result",
					},
					Edges: []wsl.Edge{
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							WhenExpr:  &wsl.Expr{Raw: "$constants.version == \"1.0.0\""},
							To:        "SuccessState",
						},
						{
							Condition: wsl.Condition{Kind: wsl.CondError},
							To:        "ErrorState",
						},
					},
				},
				"SuccessState": {
					Name:         "SuccessState",
					Terminal:     true,
					TerminalKind: "ok",
				},
				"ErrorState": {
					Name:         "ErrorState",
					Terminal:     true,
					TerminalKind: "fail",
				},
			},
		}

		// Convert WSL Graph to schema
		schema := wslGraphToSchema(graph)

		// Verify schema structure
		if schema == nil {
			t.Fatal("Schema should not be nil")
		}

		// Check transitions
		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("Transitions should be a slice of maps")
		}

		// Find the start state transition
		var startTransition map[string]interface{}
		for _, tr := range transitions {
			if name, ok := tr["name"].(string); ok && name == "StartState" {
				startTransition = tr
				break
			}
		}

		if startTransition == nil {
			t.Fatal("Start transition not found")
		}

		// Verify on_success_when is set
		oswValue, ok := startTransition["on_success_when"]
		if !ok {
			t.Error("on_success_when should be present in transition")
		}

		oswStr, ok := oswValue.(string)
		if !ok {
			t.Error("on_success_when should be a string")
		}

		expectedCondition := "$constants.version == \"1.0.0\""
		if oswStr != expectedCondition {
			t.Errorf("Expected on_success_when to be '%s', got '%s'", expectedCondition, oswStr)
		}

		// Verify true path is set
		truePath, ok := startTransition["true"].(string)
		if !ok {
			t.Error("true path should be set")
		}

		if truePath == "" {
			t.Error("true path should not be empty")
		}

		// Verify false path is set
		falsePath, ok := startTransition["false"].(string)
		if !ok {
			t.Error("false path should be set")
		}

		if falsePath == "" {
			t.Error("false path should not be empty")
		}
	})

	t.Run("Multiple on success when conditions", func(t *testing.T) {
		// Simulate a WSL Graph with multiple on success when conditions
		graph := &wsl.Graph{
			WorkflowName: "test_workflow",
			Start:        "CheckVersion",
			Constants: map[string]interface{}{
				"version": "1.0.0",
			},
			Nodes: map[string]*wsl.Node{
				"CheckVersion": {
					Name:  "CheckVersion",
					Start: true,
					Action: &wsl.Action{
						Module: "services/common",
						Name:   "CheckVersion",
						As:     "versionCheck",
					},
					Edges: []wsl.Edge{
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							WhenExpr:  &wsl.Expr{Raw: "$constants.version == \"1.0.0\""},
							To:        "VersionOne",
						},
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							WhenExpr:  &wsl.Expr{Raw: "$constants.version == \"2.0.0\""},
							To:        "VersionTwo",
						},
						{
							Condition: wsl.Condition{Kind: wsl.CondSuccess},
							To:        "DefaultVersion",
						},
					},
				},
				"VersionOne": {
					Name:         "VersionOne",
					Terminal:     true,
					TerminalKind: "ok",
				},
				"VersionTwo": {
					Name:         "VersionTwo",
					Terminal:     true,
					TerminalKind: "ok",
				},
				"DefaultVersion": {
					Name:         "DefaultVersion",
					Terminal:     true,
					TerminalKind: "ok",
				},
			},
		}

		// Convert WSL Graph to schema
		schema := wslGraphToSchema(graph)

		// Verify schema structure
		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("Transitions should be a slice of maps")
		}

		// Find the CheckVersion transition
		var checkVersionTransition map[string]interface{}
		for _, tr := range transitions {
			if name, ok := tr["name"].(string); ok && name == "CheckVersion" {
				checkVersionTransition = tr
				break
			}
		}

		if checkVersionTransition == nil {
			t.Fatal("CheckVersion transition not found")
		}

		// Verify on_success_when captures the first condition
		oswValue, ok := checkVersionTransition["on_success_when"]
		if !ok {
			t.Error("on_success_when should be present")
		}

		oswStr, ok := oswValue.(string)
		if !ok {
			t.Error("on_success_when should be a string")
		}

		// Should capture the first on success when condition
		expectedCondition := "$constants.version == \"1.0.0\""
		if oswStr != expectedCondition {
			t.Errorf("Expected first on_success_when to be '%s', got '%s'", expectedCondition, oswStr)
		}
	})

	t.Run("Regular on success without when condition", func(t *testing.T) {
		// Simulate a WSL Graph with regular on success (no when)
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
						{
							Condition: wsl.Condition{Kind: wsl.CondError},
							To:        "ErrorState",
						},
					},
				},
				"NextState": {
					Name:         "NextState",
					Terminal:     true,
					TerminalKind: "ok",
				},
				"ErrorState": {
					Name:         "ErrorState",
					Terminal:     true,
					TerminalKind: "fail",
				},
			},
		}

		// Convert WSL Graph to schema
		schema := wslGraphToSchema(graph)

		transitions, ok := schema["transitions"].([]map[string]interface{})
		if !ok {
			t.Fatal("Transitions should be a slice of maps")
		}

		// Find the SimpleState transition
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

		// Verify on_success_when is NOT set for regular on success
		_, ok = simpleTransition["on_success_when"]
		if ok {
			t.Error("on_success_when should not be present for regular on success")
		}

		// But true/false paths should still be set
		if _, ok := simpleTransition["true"]; !ok {
			t.Error("true path should be set")
		}

		if _, ok := simpleTransition["false"]; !ok {
			t.Error("false path should be set")
		}
	})
}

func TestWSLOnSuccessWhen_FromMap(t *testing.T) {
	t.Run("FlowTransition should parse on_success_when from map", func(t *testing.T) {
		// Create a map that simulates what comes from WSL conversion
		transitionMap := map[string]interface{}{
			"name":            "test_transition",
			"to":              "next_state",
			"from":            []string{"_"},
			"on_success_when": "$result.status == 'ok'",
			"true":            "success_state",
			"false":           "failure_state",
		}

		// Create a Flow and parse the transition
		flow := &domain.Flow{}
		flow.Transitions = []*domain.FlowTransition{}

		// Manually create transition from map (simulating FromMap behavior)
		transition := &domain.FlowTransition{}
		if name, ok := transitionMap["name"].(string); ok {
			transition.Name = name
		}
		if to, ok := transitionMap["to"].(string); ok {
			transition.To = to
		}
		if osw, ok := transitionMap["on_success_when"].(string); ok {
			transition.OnSuccessWhen = &osw
		}
		if truePath, ok := transitionMap["true"].(string); ok {
			transition.True = truePath
		}
		if falsePath, ok := transitionMap["false"].(string); ok {
			transition.False = falsePath
		}

		// Verify the transition was created correctly
		if transition.OnSuccessWhen == nil {
			t.Fatal("OnSuccessWhen should not be nil")
		}

		expectedCondition := "$result.status == 'ok'"
		if *transition.OnSuccessWhen != expectedCondition {
			t.Errorf("Expected OnSuccessWhen to be '%s', got '%s'", expectedCondition, *transition.OnSuccessWhen)
		}

		if transition.True != "success_state" {
			t.Errorf("Expected True to be 'success_state', got '%s'", transition.True)
		}

		if transition.False != "failure_state" {
			t.Errorf("Expected False to be 'failure_state', got '%s'", transition.False)
		}
	})
}
