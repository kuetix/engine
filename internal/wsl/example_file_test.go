package wsl

import (
	"os"
	"testing"
)

func TestParseCustomTypesExample(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/custom_types_example.wsl")
	if err != nil {
		t.Fatalf("Failed to read example file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "example" {
		t.Errorf("Expected module name 'example', got '%s'", ast.Name)
	}

	if len(graphs) != 2 {
		t.Errorf("Expected 2 workflows, got %d", len(graphs))
	}

	featureGraph, ok := graphs["example_feature"]
	if !ok {
		t.Error("Workflow 'example_feature' not found")
	} else {
		if featureGraph.WorkflowType != "feature" {
			t.Errorf("Expected workflow type 'feature', got '%s'", featureGraph.WorkflowType)
		}
		if featureGraph.WorkflowName != "example_feature" {
			t.Errorf("Expected workflow name 'example_feature', got '%s'", featureGraph.WorkflowName)
		}
	}

	appGraph, ok := graphs["example_app"]
	if !ok {
		t.Error("Workflow 'example_app' not found")
	} else {
		if appGraph.WorkflowType != "solution" {
			t.Errorf("Expected workflow type 'solution', got '%s'", appGraph.WorkflowType)
		}
		if appGraph.WorkflowName != "example_app" {
			t.Errorf("Expected workflow name 'example_app', got '%s'", appGraph.WorkflowName)
		}
	}
}

func TestParseExampleFile(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/example.wsl")
	if err != nil {
		t.Fatalf("Failed to read example file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "example" {
		t.Errorf("Expected module name 'example', got '%s'", ast.Name)
	}

	// example.wsl defines: example_solution (solution), example_project (feature),
	// example, finish_example, wsl_hello_world, workflow_name (workflows)
	if len(graphs) != 6 {
		t.Errorf("Expected 6 workflows, got %d", len(graphs))
	}

	// Verify solution type
	solutionGraph, ok := graphs["example_solution"]
	if !ok {
		t.Error("Workflow 'example_solution' not found")
	} else {
		if solutionGraph.WorkflowType != "solution" {
			t.Errorf("Expected type 'solution', got '%s'", solutionGraph.WorkflowType)
		}
		if solutionGraph.Start != "CheckSystem" {
			t.Errorf("Expected start 'CheckSystem', got '%s'", solutionGraph.Start)
		}
	}

	// Verify feature type
	featureGraph, ok := graphs["example_project"]
	if !ok {
		t.Error("Workflow 'example_project' not found")
	} else {
		if featureGraph.WorkflowType != "feature" {
			t.Errorf("Expected type 'feature', got '%s'", featureGraph.WorkflowType)
		}
	}

	// Verify standard workflow
	workflowGraph, ok := graphs["example"]
	if !ok {
		t.Error("Workflow 'example' not found")
	} else {
		if workflowGraph.WorkflowType != "workflow" {
			t.Errorf("Expected type 'workflow', got '%s'", workflowGraph.WorkflowType)
		}
		if workflowGraph.Start != "CheckSystem" {
			t.Errorf("Expected start 'CheckSystem', got '%s'", workflowGraph.Start)
		}
		if len(workflowGraph.Nodes) != 3 {
			t.Errorf("Expected 3 nodes, got %d", len(workflowGraph.Nodes))
		}
	}

	// Verify constants are available on graphs
	exampleGraph := graphs["example"]
	if exampleGraph != nil && len(exampleGraph.Constants) == 0 {
		t.Error("Expected constants to be populated on graph")
	}
}

func TestParseWhenExamplesFile(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/when_examples.wsl")
	if err != nil {
		t.Fatalf("Failed to read when_examples file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "when_examples" {
		t.Errorf("Expected module name 'when_examples', got '%s'", ast.Name)
	}

	if len(graphs) != 1 {
		t.Errorf("Expected 1 workflow, got %d", len(graphs))
	}

	graph, ok := graphs["conditional_example"]
	if !ok {
		t.Fatal("Workflow 'conditional_example' not found")
	}

	if graph.WorkflowType != "workflow" {
		t.Errorf("Expected type 'workflow', got '%s'", graph.WorkflowType)
	}

	if graph.Start != "CheckVersion" {
		t.Errorf("Expected start 'CheckVersion', got '%s'", graph.Start)
	}

	// Verify constants are present
	if graph.Constants == nil {
		t.Fatal("Expected constants to be populated")
	}
	if graph.Constants["version"] != "1.0.0" {
		t.Errorf("Expected constant 'version'='1.0.0', got '%v'", graph.Constants["version"])
	}
	if graph.Constants["maxRetries"] != int64(3) {
		t.Errorf("Expected constant 'maxRetries'=3, got '%v'", graph.Constants["maxRetries"])
	}
	if graph.Constants["enabled"] != true {
		t.Errorf("Expected constant 'enabled'=true, got '%v'", graph.Constants["enabled"])
	}

	// Verify CheckVersion node has when expressions on its success edges
	checkVersionNode := graph.Nodes["CheckVersion"]
	if checkVersionNode == nil {
		t.Fatal("Node 'CheckVersion' not found")
	}

	// Should have 3 outgoing success edges (2 with when, 1 without)
	if len(checkVersionNode.Edges) != 3 {
		t.Fatalf("Expected 3 edges on CheckVersion, got %d", len(checkVersionNode.Edges))
	}

	// First two edges should have WhenExpr
	if checkVersionNode.Edges[0].WhenExpr == nil {
		t.Error("CheckVersion edge[0] should have a when expression")
	}
	if checkVersionNode.Edges[1].WhenExpr == nil {
		t.Error("CheckVersion edge[1] should have a when expression")
	}
	// Third edge is the plain 'on success' fallback
	if checkVersionNode.Edges[2].WhenExpr != nil {
		t.Error("CheckVersion edge[2] should not have a when expression")
	}
}

func TestParseAttributeExamplesFile(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/attribute_examples.wsl")
	if err != nil {
		t.Fatalf("Failed to read attribute_examples file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "test_attributes" {
		t.Errorf("Expected module name 'test_attributes', got '%s'", ast.Name)
	}

	if len(graphs) != 1 {
		t.Errorf("Expected 1 workflow, got %d", len(graphs))
	}

	graph, ok := graphs["attribute_test"]
	if !ok {
		t.Fatal("Workflow 'attribute_test' not found")
	}

	// Verify ConditionalState has IfExpr set
	conditionalNode := graph.Nodes["ConditionalState"]
	if conditionalNode == nil {
		t.Fatal("Node 'ConditionalState' not found")
	}
	if conditionalNode.IfExpr == nil {
		t.Error("ConditionalState should have IfExpr set")
	}

	// Verify ContinueOnFailState has ContinueOnFail set
	continueNode := graph.Nodes["ContinueOnFailState"]
	if continueNode == nil {
		t.Fatal("Node 'ContinueOnFailState' not found")
	}
	if !continueNode.ContinueOnFail {
		t.Error("ContinueOnFailState should have ContinueOnFail=true")
	}

	// Verify SkipToState has SkipTo set
	skipNode := graph.Nodes["SkipToState"]
	if skipNode == nil {
		t.Fatal("Node 'SkipToState' not found")
	}
	if !skipNode.SkipTo {
		t.Error("SkipToState should have SkipTo=true")
	}

	// Verify CombinedAttributesState has both IfExpr and on-success-when edges
	combinedNode := graph.Nodes["CombinedAttributesState"]
	if combinedNode == nil {
		t.Fatal("Node 'CombinedAttributesState' not found")
	}
	if combinedNode.IfExpr == nil {
		t.Error("CombinedAttributesState should have IfExpr set")
	}
	if len(combinedNode.Edges) != 2 {
		t.Fatalf("CombinedAttributesState expected 2 edges, got %d", len(combinedNode.Edges))
	}
	if combinedNode.Edges[0].WhenExpr == nil {
		t.Error("CombinedAttributesState edge[0] should have a when expression (on success when)")
	}
	if combinedNode.Edges[1].WhenExpr != nil {
		t.Error("CombinedAttributesState edge[1] should not have a when expression")
	}
}

func TestParseOrchestrationExampleFile(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/orchestration_example.wsl")
	if err != nil {
		t.Fatalf("Failed to read orchestration_example file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "orchestration_example" {
		t.Errorf("Expected module name 'orchestration_example', got '%s'", ast.Name)
	}

	// orchestration_example.wsl defines:
	// workflows: basic_step, process_data, validate_results
	// features:  data_processing_feature, reporting_feature
	// solutions: complete_solution
	if len(graphs) != 6 {
		t.Errorf("Expected 6 workflows/features/solutions, got %d", len(graphs))
	}

	// Verify workflow types
	for name, wfType := range map[string]string{
		"basic_step":              "workflow",
		"process_data":            "workflow",
		"validate_results":        "workflow",
		"data_processing_feature": "feature",
		"reporting_feature":       "feature",
		"complete_solution":       "solution",
	} {
		g, ok := graphs[name]
		if !ok {
			t.Errorf("Expected graph '%s' not found", name)
			continue
		}
		if g.WorkflowType != wfType {
			t.Errorf("Graph '%s': expected type '%s', got '%s'", name, wfType, g.WorkflowType)
		}
	}
}

func TestParseOrchestrationWithPathsFile(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/orchestration_with_paths.wsl")
	if err != nil {
		t.Fatalf("Failed to read orchestration_with_paths file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "orchestration_with_paths" {
		t.Errorf("Expected module name 'orchestration_with_paths', got '%s'", ast.Name)
	}

	// orchestration_with_paths.wsl defines:
	// workflows: simple_task, test_solution_with_path, test_feature_with_path
	// features:  processing_feature
	// solutions: complete_solution
	if len(graphs) != 5 {
		t.Errorf("Expected 5 graphs, got %d", len(graphs))
	}

	for name, wfType := range map[string]string{
		"simple_task":             "workflow",
		"test_solution_with_path": "workflow",
		"test_feature_with_path":  "workflow",
		"processing_feature":      "feature",
		"complete_solution":       "solution",
	} {
		g, ok := graphs[name]
		if !ok {
			t.Errorf("Expected graph '%s' not found", name)
			continue
		}
		if g.WorkflowType != wfType {
			t.Errorf("Graph '%s': expected type '%s', got '%s'", name, wfType, g.WorkflowType)
		}
	}
}

func TestParseFeatureTestFile(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/feature_test.wsl")
	if err != nil {
		t.Fatalf("Failed to read feature_test file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "feature_test" {
		t.Errorf("Expected module name 'feature_test', got '%s'", ast.Name)
	}

	if len(graphs) != 2 {
		t.Errorf("Expected 2 graphs, got %d", len(graphs))
	}

	workflowGraph, ok := graphs["basic_workflow"]
	if !ok {
		t.Error("Graph 'basic_workflow' not found")
	} else if workflowGraph.WorkflowType != "workflow" {
		t.Errorf("Expected type 'workflow', got '%s'", workflowGraph.WorkflowType)
	}

	featureGraph, ok := graphs["feature_test"]
	if !ok {
		t.Error("Graph 'feature_test' not found")
	} else if featureGraph.WorkflowType != "feature" {
		t.Errorf("Expected type 'feature', got '%s'", featureGraph.WorkflowType)
	}
}

func TestParseSolutionTestFile(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/solution_test.wsl")
	if err != nil {
		t.Fatalf("Failed to read solution_test file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "solution_test" {
		t.Errorf("Expected module name 'solution_test', got '%s'", ast.Name)
	}

	if len(graphs) != 3 {
		t.Errorf("Expected 3 graphs, got %d", len(graphs))
	}

	for name, wfType := range map[string]string{
		"step1":         "workflow",
		"step2":         "workflow",
		"solution_test": "solution",
	} {
		g, ok := graphs[name]
		if !ok {
			t.Errorf("Expected graph '%s' not found", name)
			continue
		}
		if g.WorkflowType != wfType {
			t.Errorf("Graph '%s': expected type '%s', got '%s'", name, wfType, g.WorkflowType)
		}
	}

	// Verify solution orchestrates the two steps
	solutionGraph := graphs["solution_test"]
	if solutionGraph != nil {
		if solutionGraph.Start != "RunStep1" {
			t.Errorf("Expected solution start 'RunStep1', got '%s'", solutionGraph.Start)
		}
		if len(solutionGraph.Nodes) != 3 {
			t.Errorf("Expected 3 nodes in solution, got %d", len(solutionGraph.Nodes))
		}
	}
}

func TestParseSolutionMixedTestFile(t *testing.T) {
	file, err := os.ReadFile("../../runtime/workflows/wsl_hello_world/solution_mixed_test.wsl")
	if err != nil {
		t.Fatalf("Failed to read solution_mixed_test file: %v", err)
	}

	ast, graphs, err := ParseAll(string(file), "")
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	if ast.Name != "solution_mixed_test" {
		t.Errorf("Expected module name 'solution_mixed_test', got '%s'", ast.Name)
	}

	if len(graphs) != 3 {
		t.Errorf("Expected 3 graphs, got %d", len(graphs))
	}

	for name, wfType := range map[string]string{
		"simple_workflow":     "workflow",
		"simple_feature":      "feature",
		"solution_mixed_test": "solution",
	} {
		g, ok := graphs[name]
		if !ok {
			t.Errorf("Expected graph '%s' not found", name)
			continue
		}
		if g.WorkflowType != wfType {
			t.Errorf("Graph '%s': expected type '%s', got '%s'", name, wfType, g.WorkflowType)
		}
	}
}
