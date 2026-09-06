package workflow

import (
	"testing"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/internal/wsl"
)

func whileFlow(cond string, max int) *domain.Flow {
	return &domain.Flow{
		Name:       "drain",
		ConfigName: "drain",
		CurrentState: &domain.FlowState{
			State: "tests/parallel/Do#Drain",
		},
		CurrentTransition: &domain.FlowTransition{
			Name:      "Drain",
			To:        "Drain#1",
			WhileMax:  max,
			WhileCond: cond,
			True:      "Done#1",
			False:     "Failed#1",
		},
		States: []*domain.FlowState{
			{State: "Drain#1", Type: domain.StateNormal},
			{State: "Done#1", Type: domain.StateFinal},
			{State: "Failed#1", Type: domain.StateFinal},
		},
	}
}

func TestProcessWhile_RunsUntilConditionFalse(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})

	ok, next := worker.ProcessState(nil, whileFlow("while_index < 3", 10))
	if !ok || next != "Done#1" {
		t.Fatalf("ok=%v next=%q, want true/Done#1 (err %v)", ok, next, worker.GetError())
	}
	if calls != 3 {
		t.Errorf("action called %d times, want 3", calls)
	}
}

func TestProcessWhile_ConditionFalseImmediately(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})

	ok, next := worker.ProcessState(nil, whileFlow("false", 10))
	if !ok || next != "Done#1" {
		t.Fatalf("ok=%v next=%q, want true/Done#1", ok, next)
	}
	if calls != 0 {
		t.Errorf("action called %d times, want 0", calls)
	}
}

func TestProcessWhile_ReachesMaxTakesFailPath(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})

	ok, next := worker.ProcessState(nil, whileFlow("while_index < 100", 3))
	if !ok || next != "Failed#1" {
		t.Fatalf("ok=%v next=%q, want true/Failed#1", ok, next)
	}
	if calls != 3 {
		t.Errorf("action called %d times, want 3 (the max)", calls)
	}
	if worker.GetError() == nil {
		t.Error("expected a 'reached max iterations' error")
	}
}

func TestProcessWhile_ActionFailureTakesFailPath(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls, failFrom: 1})

	ok, next := worker.ProcessState(nil, whileFlow("while_index < 10", 10))
	if !ok || next != "Failed#1" {
		t.Fatalf("ok=%v next=%q, want true/Failed#1", ok, next)
	}
	if calls != 2 {
		t.Errorf("action called %d times, want 2 (stop on first failure)", calls)
	}
}

func TestWhile_SchemaPipeline(t *testing.T) {
	src := `
module drain

workflow drain {
  start: Drain

  state Drain {
    while[max: 500] <<queue.size>> > 0 {
      action queue/queue.Pop() as item
    }
    on success -> Done
    on fail -> Failed
  }

  state Done { end ok }
  state Failed { end fail }
}
`
	_, graphs, err := wsl.ParseAll(src, "drain")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	flow := &domain.Flow{Name: "drain", ConfigName: "drain", Properties: &domain.FlowOptions{}}
	if err := flow.FromMap(wslGraphToSchema(graphs["drain"])); err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	if err := (&Engine{}).CorrectFlow(flow); err != nil {
		t.Fatalf("CorrectFlow: %v", err)
	}
	var d *domain.FlowTransition
	for _, tr := range flow.Transitions {
		if tr.Name == "Drain" {
			d = tr
		}
	}
	if d == nil {
		t.Fatal("Drain transition not found")
	}
	if d.WhileMax != 500 || d.WhileCond != "<<queue.size>> > 0" {
		t.Errorf("while: max=%d cond=%q", d.WhileMax, d.WhileCond)
	}
	if d.True != "Done#1" || d.False != "Failed#1" {
		t.Errorf("routing: true=%q false=%q", d.True, d.False)
	}
}

func TestBuildAST_WhileValidation(t *testing.T) {
	bad := []struct{ name, body string }{
		{"missing max", `while[foo: 1] x > 0 { action a/a.Do() as r }`},
		{"max zero", `while[max: 0] x > 0 { action a/a.Do() as r }`},
		{"malformed cond", `while[max: 3] x + { action a/a.Do() as r }`},
		{"with top-level action", "action a/a.X()\n    while[max: 3] x > 0 { action a/a.Do() as r }"},
	}
	for _, c := range bad {
		src := "module m\nworkflow m {\n start: S\n state S {\n  " + c.body + `
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}`
		if _, _, err := wsl.ParseAll(src, "m"); err == nil {
			t.Errorf("%s: expected a build error", c.name)
		}
	}
}
