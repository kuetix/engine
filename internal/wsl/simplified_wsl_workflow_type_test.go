package wsl

import (
	"testing"
)

func TestParseSimplifiedWSL_FeatureType(t *testing.T) {
	src := `
module example

feature

action.Call(param: "value") -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if ast.Name != "example" {
		t.Errorf("expected module name 'example', got '%s'", ast.Name)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "feature" {
		t.Errorf("expected workflow type 'feature', got '%s'", wf.Type)
	}

	// Name should default to "main" when not explicitly specified
	if wf.Name != "main" {
		t.Errorf("expected workflow name 'main', got '%s'", wf.Name)
	}
}

func TestParseSimplifiedWSL_SolutionType(t *testing.T) {
	src := `
module example

solution

action.Call(param: "value") -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "solution" {
		t.Errorf("expected workflow type 'solution', got '%s'", wf.Type)
	}
}

func TestParseSimplifiedWSL_CustomType(t *testing.T) {
	src := `
module example

microservice

action.Call(param: "value") -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "microservice" {
		t.Errorf("expected workflow type 'microservice', got '%s'", wf.Type)
	}
}

func TestParseSimplifiedWSL_FeatureTypeWithName(t *testing.T) {
	src := `
module example

feature my_feature

action.Call(param: "value") -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "feature" {
		t.Errorf("expected workflow type 'feature', got '%s'", wf.Type)
	}

	if wf.Name != "my_feature" {
		t.Errorf("expected workflow name 'my_feature', got '%s'", wf.Name)
	}
}

func TestParseSimplifiedWSL_SolutionTypeWithName(t *testing.T) {
	src := `
module example

solution payment_solution

action.Call(param: "value") -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "solution" {
		t.Errorf("expected workflow type 'solution', got '%s'", wf.Type)
	}

	if wf.Name != "payment_solution" {
		t.Errorf("expected workflow name 'payment_solution', got '%s'", wf.Name)
	}
}

func TestParseSimplifiedWSL_WorkflowKeyword(t *testing.T) {
	src := `
module example

workflow

action.Call(param: "value") -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "workflow" {
		t.Errorf("expected workflow type 'workflow', got '%s'", wf.Type)
	}
}

func TestParseSimplifiedWSL_NoTypeSpecified(t *testing.T) {
	src := `
module example

action.Call(param: "value") -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	// Default should be "workflow"
	if wf.Type != "workflow" {
		t.Errorf("expected default workflow type 'workflow', got '%s'", wf.Type)
	}
}

func TestParseSimplifiedWSL_FeatureWithConstants(t *testing.T) {
	src := `
module example

const {
	timeout: 5000
}

feature

action.Call(timeout: $constants.timeout) -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if len(ast.Constants) != 1 {
		t.Fatalf("expected 1 constant, got %d", len(ast.Constants))
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "feature" {
		t.Errorf("expected workflow type 'feature', got '%s'", wf.Type)
	}
}

func TestParseSimplifiedWSL_FeatureWithErrorHandler(t *testing.T) {
	src := `
module example

feature

def errors.OnAnyError(msg: "error") as errorHandler -> .

action.Call(param: "value") <- errorHandler -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	wf := ast.Workflows[0]
	if wf.Type != "feature" {
		t.Errorf("expected workflow type 'feature', got '%s'", wf.Type)
	}
}

func TestParseAllSimplified_FeatureType(t *testing.T) {
	src := `
module payment_example

import services/common

const {
    timeout: 5000
}

feature payment_feature

payment.Process(timeout: $constants.timeout) -> common.Response(status: "ok") -> .
`

	ast, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if ast.Name != "payment_example" {
		t.Errorf("expected module name 'payment_example', got '%s'", ast.Name)
	}

	if len(graphs) != 1 {
		t.Fatalf("expected 1 graph, got %d", len(graphs))
	}

	// Graph should be keyed by workflow name
	graph, ok := graphs["payment_feature"]
	if !ok {
		t.Fatal("payment_feature workflow graph not found")
	}

	if graph.WorkflowType != "feature" {
		t.Errorf("expected workflow type 'feature', got '%s'", graph.WorkflowType)
	}
}

func TestParseAllSimplified_SolutionType(t *testing.T) {
	src := `
module auth_example

solution auth_solution

action.Authenticate() -> action.Authorize() -> .
`

	_, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if len(graphs) != 1 {
		t.Fatalf("expected 1 graph, got %d", len(graphs))
	}

	graph, ok := graphs["auth_solution"]
	if !ok {
		t.Fatal("auth_solution workflow graph not found")
	}

	if graph.WorkflowType != "solution" {
		t.Errorf("expected workflow type 'solution', got '%s'", graph.WorkflowType)
	}
}

func TestParseSimplifiedWSL_ComplexFeatureFlow(t *testing.T) {
	src := `
module complex_flow

import services/common
import billing/payment

const {
    maxRetries: 3,
    timeoutMs: 5000
}

feature payment_processing

handlers.LogError(level: "error") as logError -> .

payment.ValidateCard(timeout: $constants.timeoutMs) as validation <- logError -> 
payment.ChargeCard(amount: 100, retries: $constants.maxRetries) as charge <- logError ->
common.Response(message: "Payment successful", statusCode: 200) -> .
`

	ast, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if ast.Name != "complex_flow" {
		t.Errorf("expected module name 'complex_flow', got '%s'", ast.Name)
	}

	if len(ast.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(ast.Imports))
	}

	if len(ast.Constants) != 2 {
		t.Fatalf("expected 2 constants, got %d", len(ast.Constants))
	}

	graph, ok := graphs["payment_processing"]
	if !ok {
		t.Fatal("payment_processing workflow graph not found")
	}

	if graph.WorkflowType != "feature" {
		t.Errorf("expected workflow type 'feature', got '%s'", graph.WorkflowType)
	}

	if len(graph.Nodes) < 3 {
		t.Errorf("expected at least 3 nodes in workflow, got %d", len(graph.Nodes))
	}
}
