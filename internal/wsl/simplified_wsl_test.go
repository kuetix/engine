package wsl

import (
	"testing"
)

func TestParseSimplifiedWSL_BasicFlow(t *testing.T) {
	src := `
module example

const {
	msg: "Hello"
}

speak.Say(on: "message") -> .
`

	mod, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if mod.Name != "example" {
		t.Errorf("expected module name 'example', got '%s'", mod.Name)
	}

	if len(mod.Constants) != 1 {
		t.Fatalf("expected 1 constant, got %d", len(mod.Constants))
	}

	if mod.Constants[0].Name != "msg" {
		t.Errorf("expected constant name 'msg', got '%s'", mod.Constants[0].Name)
	}

	if len(mod.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(mod.Workflows))
	}

	wf := mod.Workflows[0]
	if wf.Name != "main" {
		t.Errorf("expected workflow name 'main', got '%s'", wf.Name)
	}
}

func TestParseSimplifiedWSL_WithErrorHandler(t *testing.T) {
	src := `
module example

errors.OnAnyError() as err -> .

speak.Say(on: "message") <- err -> .
`

	mod, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if mod.Name != "example" {
		t.Errorf("expected module name 'example', got '%s'", mod.Name)
	}

	if len(mod.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(mod.Workflows))
	}
}

func TestParseSimplifiedWSL_ChainedActions(t *testing.T) {
	src := `
module example

speak.Say(on: "message") -> common.Response(status: 200) -> .
`

	mod, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if mod.Name != "example" {
		t.Errorf("expected module name 'example', got '%s'", mod.Name)
	}

	if len(mod.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(mod.Workflows))
	}
}

func TestParseSimplifiedWSL_WithImports(t *testing.T) {
	src := `
module example

import services/common

const {
	msg: "Hello"
}

speak.Say(on: "message") -> .
`

	mod, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if mod.Name != "example" {
		t.Errorf("expected module name 'example', got '%s'", mod.Name)
	}

	if len(mod.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(mod.Imports))
	}

	if mod.Imports[0].Path != "services/common" {
		t.Errorf("expected import path 'services/common', got '%s'", mod.Imports[0].Path)
	}
}

func TestLexer_LeftArrow(t *testing.T) {
	src := "speak.Say() <- errorHandler -> ."
	lx := NewLexer(src)

	tokens := []TokenKind{TokIdent, TokLParen, TokRParen, TokLeftArrow, TokIdent, TokArrow, TokDot}

	for i, expected := range tokens {
		tok := lx.Next()
		if tok.Kind != expected {
			t.Errorf("token %d: expected %s, got %s", i, expected, tok.Kind)
		}
	}
}

func TestParseAllSimplified(t *testing.T) {
	src := `
module example

const {
	msg: "Hello World",
	code: 200
}

errors.OnAnyError() as errorHandler -> .

speak.Say(on: "message", v: $constants.msg) as response <- errorHandler -> common.Response(message: $response.message, statusCode: $constants.code) -> .
`

	ast, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if ast.Name != "example" {
		t.Errorf("expected module name 'example', got '%s'", ast.Name)
	}

	if len(ast.Constants) != 2 {
		t.Fatalf("expected 2 constants, got %d", len(ast.Constants))
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	if len(graphs) != 1 {
		t.Fatalf("expected 1 graph, got %d", len(graphs))
	}

	// Verify the main workflow exists
	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	if mainGraph.WorkflowName != "main" {
		t.Errorf("expected workflow name 'main', got '%s'", mainGraph.WorkflowName)
	}
}

func TestParseComplexSimplifiedWorkflow(t *testing.T) {
	src := `
module complex_flow

import services/common
import billing/payment

const {
    maxRetries: 3,
    timeoutMs: 5000
}

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

	// Verify the main workflow
	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	// Should have multiple states for the chained actions
	if len(mainGraph.Nodes) < 3 {
		t.Errorf("expected at least 3 nodes in workflow, got %d", len(mainGraph.Nodes))
	}
}

func TestParseSimplifiedWSL_WithDef(t *testing.T) {
	src := `
module example

const {
	msg: "Hello World",
	code: 200
}

def services/common/errors.OnAnyError(msg: "error", statusCode: 400) as errorHandler -> .

converse/speak.Say(on: "message", v: $constants.msg) as response <- errorHandler -> services/common/response.Response(message: $response.message, statusCode: $constants.code) -> .
def services/common/errors.OnAnyError(msg: "error", statusCode: 400) as errorHandler -> .

converse/speak.Say(on: "message", v: $constants.msg) as response <- errorHandler -> services/common/response.Response(message: $response.message, statusCode: $constants.code) -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if ast.Name != "example" {
		t.Errorf("expected module name 'example', got '%s'", ast.Name)
	}
}

func TestParseSimplifiedWSL_WithInlineErrorAction(t *testing.T) {
	src := `
module example

converse/speak.Say(on: "message") <- services/common/errors.OnAnyError(msg: "error") -> .
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
}

func TestParseSimplifiedWSL_WithNestedStructures(t *testing.T) {
	src := `
module test

const {
    nested: {
        inner: {
            value: "test"
        }
    }
}

action.Call(
    simple: "value",
    nested_obj: {
        key1: "value1",
        key2: {
            deep: "value"
        }
    },
    nested_array: [
        "item1",
        {obj: "in_array"},
        ["nested_array"]
    ]
) -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	if ast.Name != "test" {
		t.Errorf("expected module name 'test', got '%s'", ast.Name)
	}

	if len(ast.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(ast.Workflows))
	}

	// Verify the workflow has states
	wf := ast.Workflows[0]
	if len(wf.States) == 0 {
		t.Fatal("expected at least one state in workflow")
	}
}

func TestParseAllSimplified_WithNestedStructures(t *testing.T) {
	src := `
module payment_example

import services/common

const {
    config: {
        timeout: 5000,
        retries: {
            max: 3,
            backoff: [100, 200, 400]
        }
    }
}

payment.Process(
    amount: 100,
    metadata: {
        user: "test@example.com",
        tags: ["priority", "verified"],
        settings: {
            notifications: true,
            limits: [500, 1000, 5000]
        }
    }
) -> common.Response(status: "ok") -> .
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

	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	if len(mainGraph.Nodes) < 2 {
		t.Errorf("expected at least 2 nodes in workflow, got %d", len(mainGraph.Nodes))
	}
}

func TestParseSimplifiedWSL_DefNotExecutedAsState(t *testing.T) {
	src := `
module test

def errors.OnAnyError(msg: "error") as errorHandler -> .

action.Call(param: "value") <- errorHandler -> .
`

	ast, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if ast.Name != "test" {
		t.Errorf("expected module name 'test', got '%s'", ast.Name)
	}

	if len(graphs) != 1 {
		t.Fatalf("expected 1 graph, got %d", len(graphs))
	}

	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	// The workflow should have 2 states:
	// 1. The main action (action.Call)
	// 2. The inlined error handler (errors.OnAnyError)
	// The def itself should NOT be a separate workflow state
	if len(mainGraph.Nodes) != 2 {
		t.Errorf("expected 2 nodes (main action + inlined error handler), got %d", len(mainGraph.Nodes))
		for name := range mainGraph.Nodes {
			t.Logf("  Node: %s", name)
		}
	}

	// Verify the start state is the main action, not the def
	if mainGraph.Start == "errorHandler" {
		t.Error("start state should not be 'errorHandler' (the def); def should be inlined, not executed first")
	}
}

func TestParseSimplifiedWSL_UserExample(t *testing.T) {
	// This is the exact example from the user's comment
	src := `
module simplified_example

import services/common

const {
    msg: "Hello World",
    code: 200
}

// Define error handler first
def services/common/errors.OnAnyError(msg: "error", statusCode: 400) as errorHandler -> .

// Main action flow with error binding
converse/speak.Say(on: "message", v: $constants.msg) as response <- errorHandler -> services/common/response.Response(message: $response.message, statusCode: $constants.code) -> .
`

	ast, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if ast.Name != "simplified_example" {
		t.Errorf("expected module name 'simplified_example', got '%s'", ast.Name)
	}

	if len(graphs) != 1 {
		t.Fatalf("expected 1 graph, got %d", len(graphs))
	}

	mainGraph := graphs["main"]

	// The workflow should have 3 states:
	// 1. response (converse/speak.Say)
	// 2. errorHandler (inlined from def when referenced)
	// 3. services/common/response.Response
	// The def itself should NOT be executed as the first state
	if len(mainGraph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(mainGraph.Nodes))
		for name := range mainGraph.Nodes {
			t.Logf("  Node: %s", name)
		}
	}

	// Verify the start state is the main action (response), not errorHandler
	if mainGraph.Start == "errorHandler" {
		t.Error("start state should not be 'errorHandler'; def should only be inlined when referenced, not executed first")
	}

	// The start should be the first real action (response)
	if mainGraph.Start != "response" {
		t.Errorf("start state should be 'response', got '%s'", mainGraph.Start)
	}
}

func TestParseSimplifiedWSL_OptionalModule_WithFilename(t *testing.T) {
	// No module keyword - should derive from filename
	src := `
const {
    msg: "Hello"
}

action.Call(value: "test") -> .
`

	ast, err := ParseSimplifiedWSLWithFilename(src, "my_workflow.swsl")
	if err != nil {
		t.Fatalf("ParseSimplifiedWSLWithFilename failed: %v", err)
	}

	// Module name should be derived from filename (without extension)
	if ast.Name != "my_workflow" {
		t.Errorf("expected module name 'my_workflow' (from filename), got '%s'", ast.Name)
	}
}

func TestParseSimplifiedWSL_OptionalModule_DefaultName(t *testing.T) {
	// No module keyword and no filename - should use default
	src := `
action.Call(value: "test") -> .
`

	ast, err := ParseSimplifiedWSL(src)
	if err != nil {
		t.Fatalf("ParseSimplifiedWSL failed: %v", err)
	}

	// Module name should be default "main"
	if ast.Name != "main" {
		t.Errorf("expected module name 'main' (default), got '%s'", ast.Name)
	}
}

func TestParseSimplifiedWSL_ExplicitModuleTakesPrecedence(t *testing.T) {
	// Explicit module keyword should take precedence over filename
	src := `
module explicit_name

action.Call(value: "test") -> .
`

	ast, err := ParseSimplifiedWSLWithFilename(src, "filename.swsl")
	if err != nil {
		t.Fatalf("ParseSimplifiedWSLWithFilename failed: %v", err)
	}

	// Module name should be from explicit declaration, not filename
	if ast.Name != "explicit_name" {
		t.Errorf("expected module name 'explicit_name' (explicit), got '%s'", ast.Name)
	}
}

func TestParseAllSimplifiedWithFilename(t *testing.T) {
	src := `
const {
    value: 100
}

payment.Process(amount: $constants.value) -> common.Response(status: "ok") -> .
`

	ast, graphs, err := ParseAllSimplifiedWithFilename(src, "payment_flow.swsl")
	if err != nil {
		t.Fatalf("ParseAllSimplifiedWithFilename failed: %v", err)
	}

	if ast.Name != "payment_flow" {
		t.Errorf("expected module name 'payment_flow', got '%s'", ast.Name)
	}

	if len(graphs) != 1 {
		t.Fatalf("expected 1 graph, got %d", len(graphs))
	}
}

// TestParseSimplifiedWSL_ErrorHandlerTerminalIsFail verifies that a def used as an
// error handler via <- has End.Kind = "fail" when it ends with -> .
func TestParseSimplifiedWSL_ErrorHandlerTerminalIsFail(t *testing.T) {
	src := `
module test

def errors.OnAnyError(msg: "error") as errorHandler -> .

action.Call(param: "value") <- errorHandler -> .
`

	_, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	if len(mainGraph.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(mainGraph.Nodes))
	}

	// The error handler node (errorHandler) should be terminal with kind "fail"
	ehNode := mainGraph.Nodes["errorHandler"]
	if ehNode == nil {
		t.Fatal("errorHandler node not found")
	}
	if !ehNode.Terminal {
		t.Error("errorHandler node should be terminal (has -> .)")
	}
	if ehNode.TerminalKind != "fail" {
		t.Errorf("errorHandler terminal kind should be 'fail', got '%s'", ehNode.TerminalKind)
	}

	// The main action should be terminal with kind "ok" (has -> .)
	mainNode := mainGraph.Nodes["action.Call"]
	if mainNode == nil {
		t.Fatal("action.Call node not found")
	}
	if !mainNode.Terminal {
		t.Error("action.Call node should be terminal (has -> .)")
	}
	if mainNode.TerminalKind != "ok" {
		t.Errorf("action.Call terminal kind should be 'ok', got '%s'", mainNode.TerminalKind)
	}
}

// TestParseSimplifiedWSL_DefSuccessBinding verifies that a def can be referenced
// via -> (success binding), not just via <- (error binding).
func TestParseSimplifiedWSL_DefSuccessBinding(t *testing.T) {
	src := `
module test

def response.SendOK(status: 200) as successHandler -> .

speak.Say(on: "message") -> successHandler
`

	ast, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if ast.Name != "test" {
		t.Errorf("expected module name 'test', got '%s'", ast.Name)
	}

	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	// Should have 2 nodes: speak.Say and successHandler
	if len(mainGraph.Nodes) != 2 {
		t.Errorf("expected 2 nodes (speak.Say + successHandler), got %d", len(mainGraph.Nodes))
		for name := range mainGraph.Nodes {
			t.Logf("  Node: %s", name)
		}
	}

	// The start state should be speak.Say (not the def)
	if mainGraph.Start == "successHandler" {
		t.Error("start state should not be 'successHandler'; def should only be inlined when referenced")
	}

	// The successHandler node should be terminal with kind "ok"
	shNode := mainGraph.Nodes["successHandler"]
	if shNode == nil {
		t.Fatal("successHandler node not found")
	}
	if !shNode.Terminal {
		t.Error("successHandler node should be terminal (has -> .)")
	}
	if shNode.TerminalKind != "ok" {
		t.Errorf("successHandler terminal kind should be 'ok', got '%s'", shNode.TerminalKind)
	}

	// The speak.Say node should have a success transition to successHandler
	sayNode := mainGraph.Nodes["speak.Say"]
	if sayNode == nil {
		t.Fatal("speak.Say node not found")
	}
	if len(sayNode.Edges) != 1 {
		t.Fatalf("speak.Say should have 1 edge, got %d", len(sayNode.Edges))
	}
	edge := sayNode.Edges[0]
	if edge.Condition.Kind != CondSuccess {
		t.Errorf("speak.Say -> successHandler edge should be success condition, got '%s'", edge.Condition.Kind)
	}
	if edge.To != "successHandler" {
		t.Errorf("speak.Say -> successHandler edge target should be 'successHandler', got '%s'", edge.To)
	}
}

// TestParseSimplifiedWSL_DefSuccessAndErrorBinding verifies that a def can be used
// both as a success handler (via ->) and an error handler (via <-) in the same workflow.
func TestParseSimplifiedWSL_DefSuccessAndErrorBinding(t *testing.T) {
	src := `
module test

def response.SendOK(status: 200) as successHandler -> .
def errors.OnAnyError(msg: "error") as errorHandler -> .

speak.Say(on: "message") <- errorHandler -> successHandler
`

	ast, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if ast.Name != "test" {
		t.Errorf("expected module name 'test', got '%s'", ast.Name)
	}

	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	// Should have 3 nodes: speak.Say, successHandler, errorHandler
	if len(mainGraph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(mainGraph.Nodes))
		for name := range mainGraph.Nodes {
			t.Logf("  Node: %s", name)
		}
	}

	// successHandler should be terminal with kind "ok"
	shNode := mainGraph.Nodes["successHandler"]
	if shNode == nil {
		t.Fatal("successHandler node not found")
	}
	if !shNode.Terminal || shNode.TerminalKind != "ok" {
		t.Errorf("successHandler: terminal=%v kind=%s (want terminal=true kind=ok)", shNode.Terminal, shNode.TerminalKind)
	}

	// errorHandler should be terminal with kind "fail"
	ehNode := mainGraph.Nodes["errorHandler"]
	if ehNode == nil {
		t.Fatal("errorHandler node not found")
	}
	if !ehNode.Terminal || ehNode.TerminalKind != "fail" {
		t.Errorf("errorHandler: terminal=%v kind=%s (want terminal=true kind=fail)", ehNode.Terminal, ehNode.TerminalKind)
	}

	// speak.Say should have both transitions
	sayNode := mainGraph.Nodes["speak.Say"]
	if sayNode == nil {
		t.Fatal("speak.Say node not found")
	}
	if len(sayNode.Edges) != 2 {
		t.Fatalf("speak.Say should have 2 edges, got %d", len(sayNode.Edges))
	}
}

// TestParseSimplifiedWSL_DotTerminalEverywhere verifies that -> . marks states as final
// whether used in a regular action, def, or inline nested action.
func TestParseSimplifiedWSL_DotTerminalEverywhere(t *testing.T) {
	src := `
module test

def errors.OnAnyError(msg: "error") as errorHandler -> .

speak.Say(on: "message") as response <- errorHandler ->
common.Response(value: $response.message) <- errorHandler -> .
`

	_, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	// errorHandler (def, error context) should be terminal with kind "fail"
	ehNode := mainGraph.Nodes["errorHandler"]
	if ehNode == nil {
		t.Fatal("errorHandler node not found")
	}
	if !ehNode.Terminal {
		t.Error("errorHandler node should be terminal (has -> .)")
	}
	if ehNode.TerminalKind != "fail" {
		t.Errorf("errorHandler TerminalKind should be 'fail', got '%s'", ehNode.TerminalKind)
	}

	// common.Response (inline action with -> .) should be terminal with kind "ok"
	var respNode *Node
	for name, n := range mainGraph.Nodes {
		if name != "response" && name != "errorHandler" {
			respNode = n
		}
	}
	if respNode == nil {
		t.Fatal("common.Response node not found")
	}
	if !respNode.Terminal {
		t.Errorf("common.Response node should be terminal (has -> .)")
	}
	if respNode.TerminalKind != "ok" {
		t.Errorf("common.Response TerminalKind should be 'ok', got '%s'", respNode.TerminalKind)
	}
}

// TestParseSimplifiedWSL_UserExactExample tests the exact example from the problem statement.
func TestParseSimplifiedWSL_UserExactExample(t *testing.T) {
	src := `
module module_example

import services/common

const {
    msg: "Hello from module example",
    code: 200
}

def services/common/errors.OnAnyError(msg: "error", statusCode: 500) as errorHandler -> .

converse/speak.Say(on: "message", v: [$constants.msg]) as response <- errorHandler ->
services/common/response.Response(value: $response.message, statusCode: $constants.code) <- errorHandler -> .
`

	ast, graphs, err := ParseAllSimplified(src, "")
	if err != nil {
		t.Fatalf("ParseAllSimplified failed: %v", err)
	}

	if ast.Name != "module_example" {
		t.Errorf("expected module name 'module_example', got '%s'", ast.Name)
	}

	mainGraph, ok := graphs["main"]
	if !ok {
		t.Fatal("main workflow graph not found")
	}

	// 3 nodes: response (speak.Say), errorHandler, and Response
	if len(mainGraph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(mainGraph.Nodes))
		for name := range mainGraph.Nodes {
			t.Logf("  Node: %s", name)
		}
	}

	// Start state is "response" (the first regular action)
	if mainGraph.Start != "response" {
		t.Errorf("start state should be 'response', got '%s'", mainGraph.Start)
	}

	// errorHandler (def, error context) should be terminal "fail"
	ehNode := mainGraph.Nodes["errorHandler"]
	if ehNode == nil {
		t.Fatal("errorHandler node not found")
	}
	if !ehNode.Terminal || ehNode.TerminalKind != "fail" {
		t.Errorf("errorHandler: terminal=%v kind=%s (want terminal=true kind=fail)", ehNode.Terminal, ehNode.TerminalKind)
	}

	// Response (inline, success path, ends with -> .) should be terminal "ok"
	var respNode *Node
	for name, n := range mainGraph.Nodes {
		if name != "response" && name != "errorHandler" {
			respNode = n
		}
	}
	if respNode == nil {
		t.Fatal("Response node not found")
	}
	if !respNode.Terminal || respNode.TerminalKind != "ok" {
		t.Errorf("Response: terminal=%v kind=%s (want terminal=true kind=ok)", respNode.Terminal, respNode.TerminalKind)
	}
}
