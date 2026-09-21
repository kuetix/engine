package workflow

import (
	"testing"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/internal/wsl"
)

func retryFlow(policy *domain.FlowRetry) *domain.Flow {
	return &domain.Flow{
		Name:       "charge",
		ConfigName: "charge",
		CurrentState: &domain.FlowState{
			State: "tests/parallel/Do#Charge",
		},
		CurrentTransition: &domain.FlowTransition{
			Name:  "Charge",
			To:    "Charge#1",
			Retry: policy,
			True:  "Confirm#1",
			False: "Failed#1",
		},
		States: []*domain.FlowState{
			{State: "Charge#1", Type: domain.StateNormal},
			{State: "Confirm#1", Type: domain.StateFinal},
			{State: "Failed#1", Type: domain.StateFinal},
		},
	}
}

func TestRetry_SucceedsAfterTransientFailures(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls, failUntil: 2})

	ok, next := worker.ProcessState(nil, retryFlow(&domain.FlowRetry{Max: 3}))
	if !ok || next != "Confirm#1" {
		t.Fatalf("ok=%v next=%q, want true/Confirm#1 (err %v)", ok, next, worker.GetError())
	}
	if calls != 3 {
		t.Errorf("action called %d times, want 3 (2 failures + 1 success)", calls)
	}
}

func TestRetry_ExhaustsAndTakesFailPath(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls, failUntil: 100})

	ok, next := worker.ProcessState(nil, retryFlow(&domain.FlowRetry{Max: 2}))
	if !ok || next != "Failed#1" {
		t.Fatalf("ok=%v next=%q, want true/Failed#1", ok, next)
	}
	if calls != 3 {
		t.Errorf("action called %d times, want 3 (1 + 2 retries)", calls)
	}
}

func TestRetry_OnGuardRejectsRetry(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{
		calls: &calls, failUntil: 100,
		failResp: map[string]interface{}{"retryable": false},
	})

	ok, next := worker.ProcessState(nil, retryFlow(&domain.FlowRetry{Max: 5, On: "err.retryable == true"}))
	if !ok || next != "Failed#1" {
		t.Fatalf("ok=%v next=%q, want true/Failed#1", ok, next)
	}
	if calls != 1 {
		t.Errorf("action called %d times, want 1 (guard rejected retry)", calls)
	}
}

func TestRetry_OnGuardAllowsRetry(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{
		calls: &calls, failUntil: 2,
		failResp: map[string]interface{}{"retryable": true},
	})

	ok, next := worker.ProcessState(nil, retryFlow(&domain.FlowRetry{Max: 5, On: "err.retryable"}))
	if !ok || next != "Confirm#1" {
		t.Fatalf("ok=%v next=%q, want true/Confirm#1 (err %v)", ok, next, worker.GetError())
	}
	if calls != 3 {
		t.Errorf("action called %d times, want 3", calls)
	}
}

func TestRetry_SchemaPipeline(t *testing.T) {
	src := `
module charge

workflow charge {
  start: Charge

  state Charge {
    retry[max: 3, delay: "10ms", on: "err.retryable == true"]
    action pay/pay.Charge(amount: 100) as ch
    on success -> Confirm
    on fail -> Failed
  }

  state Confirm { end ok }
  state Failed { end fail }
}
`
	_, graphs, err := wsl.ParseAll(src, "charge")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	flow := &domain.Flow{Name: "charge", ConfigName: "charge", Properties: &domain.FlowOptions{}}
	if err := flow.FromMap(wslGraphToSchema(graphs["charge"])); err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	if err := (&Engine{}).CorrectFlow(flow); err != nil {
		t.Fatalf("CorrectFlow: %v", err)
	}
	var ch *domain.FlowTransition
	for _, tr := range flow.Transitions {
		if tr.Name == "Charge" {
			ch = tr
		}
	}
	if ch == nil || ch.Retry == nil {
		t.Fatalf("Charge transition retry policy missing: %+v", ch)
	}
	if ch.Retry.Max != 3 || ch.Retry.Delay != "10ms" || ch.Retry.On != `err.retryable == true` {
		t.Errorf("retry policy = %+v", ch.Retry)
	}
}

func TestBuildAST_RetryValidation(t *testing.T) {
	bad := []string{
		`retry[delay: "1s"]`,           // missing max
		`retry[max: 0]`,                // max < 1
		`retry[max: 2, delay: "nope"]`, // bad duration
		`retry[max: 2, bogus: 1]`,      // unknown attr
		`retry[max: 2, on: "a +"]`,     // malformed expr
	}
	for _, attr := range bad {
		src := "module m\nworkflow m {\n start: S\n state S {\n  " + attr + `
    action a/a.Do() as r
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}`
		if _, _, err := wsl.ParseAll(src, "m"); err == nil {
			t.Errorf("%q: expected a build error", attr)
		}
	}
}

func TestBuildAST_RetryRejectedOnForeach(t *testing.T) {
	src := `
module m
workflow m {
  start: S
  state S {
    retry[max: 2]
    foreach x in <<xs>> {
      action a/a.Do(v: x) as r
    }
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}`
	if _, _, err := wsl.ParseAll(src, "m"); err == nil {
		t.Fatal("expected an error: retry on a foreach state")
	}
}
