package wsl

import (
	"strings"
	"testing"
)

func fullExample() string {
	return `
module billing.payment

import common.http as http
import billing.shared as shared

context {
    orderId: string
    amount:  number
    userId:  string
    status:  string
}

workflow charge_customer {
    start: validate_input

    state validate_input {
        action shared.validateOrder(orderId)
        on success -> check_balance
        on error   -> fail_invalid_order
    }

    state check_balance {
        action http.getBalance(userId) as balance
        on (balance >= amount) -> reserve_funds
        on else                -> fail_no_funds
    }

    state reserve_funds {
        action http.reserve(userId, amount)
        on success -> mark_paid
        on error   -> fail_reserve
    }

    state mark_paid {
        action shared.markPaid(orderId)
        on success -> done
        on error   -> fail_mark
    }

    state fail_invalid_order { end fail code="INVALID_ORDER" }
    state fail_no_funds      { end fail code="NO_FUNDS" }
    state fail_reserve       { end fail code="RESERVE_ERROR" }
    state fail_mark          { end fail code="MARK_ERROR" }
    state done               { end ok }
}
`
}

func TestParseCST_FullExample(t *testing.T) {
	cst, err := ParseCST(fullExample(), "")
	if err != nil {
		t.Fatalf("ParseCST error: %v", err)
	}
	if cst.NameTok.Lexeme != "billing.payment" {
		t.Fatalf("module name: got %q", cst.NameTok.Lexeme)
	}
	if len(cst.Imports) != 2 {
		t.Fatalf("imports: got %d", len(cst.Imports))
	}
	if cst.Context == nil {
		t.Fatalf("context is nil")
	}
	if got := len(cst.Context.Fields); got != 4 {
		t.Fatalf("context fields: got %d", got)
	}
	if got := len(cst.Workflows); got != 1 {
		t.Fatalf("workflows: got %d", got)
	}

	wf := cst.Workflows[0]
	if wf.NameTok.Lexeme != "charge_customer" {
		t.Fatalf("workflow name: %q", wf.NameTok.Lexeme)
	}
	if wf.StartName.Lexeme != "validate_input" {
		t.Fatalf("start: %q", wf.StartName.Lexeme)
	}

	if got := len(wf.States); got != 9 {
		t.Fatalf("states: got %d, want 9", got)
	}

	// Check one regular state
	var stValidate *CSTState
	for i := range wf.States {
		if wf.States[i].NameTok.Lexeme == "validate_input" {
			stValidate = &wf.States[i]
			break
		}
	}
	if stValidate == nil {
		t.Fatalf("state validate_input not found")
	}
	if stValidate.Action == nil {
		t.Fatalf("validate_input should have action")
	}
	if got := len(stValidate.Transitions); got != 2 {
		t.Fatalf("validate_input transitions: %d", got)
	}

	// Terminal state should have End and no transitions
	var stFail *CSTState
	for i := range wf.States {
		if wf.States[i].NameTok.Lexeme == "fail_no_funds" {
			stFail = &wf.States[i]
			break
		}
	}
	if stFail == nil {
		t.Fatalf("state fail_no_funds not found")
	}
	if stFail.End == nil {
		t.Fatalf("fail_no_funds must have end")
	}
	if len(stFail.Transitions) != 0 {
		t.Fatalf("terminal must not have transitions")
	}
}

func TestParseCST_DuplicateContext_Error(t *testing.T) {
	src := `module m
context { a: string }
context { b: string }
workflow w { start: s
state s { end ok }
}`
	_, err := ParseCST(src, "")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate context block") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseCST_ArraysInActions(t *testing.T) {
	src := `module test_arrays

workflow test {
  start: process
  
  state process {
    action setup([1, 2, 3], {data: ["a", "b"]})
    on success -> done
  }
  
  state done {
    end ok
  }
}`

	cst, err := ParseCST(src, "")
	if err != nil {
		t.Fatalf("ParseCST error: %v", err)
	}

	if len(cst.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(cst.Workflows))
	}

	wf := cst.Workflows[0]
	var processState *CSTState
	for i := range wf.States {
		if wf.States[i].NameTok.Lexeme == "process" {
			processState = &wf.States[i]
			break
		}
	}

	if processState == nil {
		t.Fatal("process state not found")
	}

	if processState.Action == nil {
		t.Fatal("process state has no action")
	}

	// Check that we have 2 args
	if len(processState.Action.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(processState.Action.Args))
	}

	// First arg should be an array literal
	arg1 := processState.Action.Args[0].Raw
	if !strings.Contains(arg1, "[") || !strings.Contains(arg1, "]") {
		t.Errorf("first arg should contain brackets: %s", arg1)
	}

	// Second arg should be an object literal with array value
	arg2 := processState.Action.Args[1].Raw
	if !strings.Contains(arg2, "{") || !strings.Contains(arg2, "}") {
		t.Errorf("second arg should contain braces: %s", arg2)
	}
	if !strings.Contains(arg2, "[") || !strings.Contains(arg2, "]") {
		t.Errorf("second arg should contain brackets: %s", arg2)
	}
}
