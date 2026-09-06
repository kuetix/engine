package workflow

import (
	"testing"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/internal/wsl"
)

func foreachFlow(list string) *domain.Flow {
	return &domain.Flow{
		Name:       "loop",
		ConfigName: "loop",
		CurrentState: &domain.FlowState{
			State: "tests/parallel/Do#PostLines",
		},
		CurrentTransition: &domain.FlowTransition{
			Name:        "PostLines",
			To:          "PostLines#1",
			ForEachVar:  "line",
			ForEachList: list,
			Response:    "posted",
			True:        "Done#1",
			False:       "Rollback#1",
		},
		States: []*domain.FlowState{
			{State: "PostLines#1", Type: domain.StateNormal},
			{State: "Done#1", Type: domain.StateFinal},
			{State: "Rollback#1", Type: domain.StateFinal},
		},
	}
}

func TestProcessForEach_AllIterationsSucceed(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})
	worker.WorkflowContext.SetValue("lines", []interface{}{"a", "b", "c"})

	ok, next := worker.ProcessState(nil, foreachFlow("lines"))
	if !ok {
		t.Fatalf("ProcessState failed: %v", worker.GetError())
	}
	if next != "Done#1" {
		t.Errorf("routed to %q, want Done#1", next)
	}
	if calls != 3 {
		t.Errorf("action called %d times, want 3", calls)
	}
	wsc := &WorkerSessionContext{WorkflowContext: worker.WorkflowContext, Worker: worker, Flow: foreachFlow("lines"), Parser: NewParser()}
	results, ok := wsc.Property("posted").([]interface{})
	if !ok || len(results) != 3 {
		t.Fatalf("alias 'posted' = %#v, want a 3-element slice", wsc.Property("posted"))
	}
	// last iteration's var + index remain bound
	if wsc.Property("line") != "c" {
		t.Errorf("line = %#v, want c", wsc.Property("line"))
	}
	if wsc.Property("line_index") != int64(2) {
		t.Errorf("line_index = %#v, want 2", wsc.Property("line_index"))
	}
}

func TestProcessForEach_EmptyList(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})
	worker.WorkflowContext.SetValue("lines", []interface{}{})

	ok, next := worker.ProcessState(nil, foreachFlow("lines"))
	if !ok || next != "Done#1" {
		t.Fatalf("empty list: ok=%v next=%q, want true/Done#1", ok, next)
	}
	if calls != 0 {
		t.Errorf("action called %d times for an empty list, want 0", calls)
	}
}

func TestProcessForEach_IterationFailureTakesFailPath(t *testing.T) {
	var calls int64
	// 2nd call onward fails.
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls, failFrom: 1})
	worker.WorkflowContext.SetValue("lines", []interface{}{"a", "b", "c"})

	ok, next := worker.ProcessState(nil, foreachFlow("lines"))
	if !ok {
		t.Fatalf("expected routing to the fail path, got hard stop: %v", worker.GetError())
	}
	if next != "Rollback#1" {
		t.Errorf("routed to %q, want Rollback#1", next)
	}
	if calls != 2 {
		t.Errorf("action called %d times, want 2 (stop after first failure)", calls)
	}
	if worker.GetError() == nil {
		t.Error("expected an error describing the failed iteration")
	}
}

func TestProcessForEach_NonListCollectionErrors(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})
	worker.WorkflowContext.SetValue("lines", "not-a-list")

	ok, _ := worker.ProcessState(nil, foreachFlow("lines"))
	if ok {
		t.Error("a non-list collection should not succeed")
	}
	if calls != 0 {
		t.Errorf("action called %d times, want 0", calls)
	}
}

func TestForEach_SchemaPipeline(t *testing.T) {
	src := `
module lines

workflow lines {
  start: PostLines

  state PostLines {
    foreach line in <<invoice.lines>> {
      action ledger/ledger.Post(amount: line.amount) as posted
    }
    on success -> Done
    on fail -> Rollback
  }

  state Done { end ok }
  state Rollback { end fail }
}
`
	_, graphs, err := wsl.ParseAll(src, "lines")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	flow := &domain.Flow{Name: "lines", ConfigName: "lines", Properties: &domain.FlowOptions{}}
	if err := flow.FromMap(wslGraphToSchema(graphs["lines"])); err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	if err := (&Engine{}).CorrectFlow(flow); err != nil {
		t.Fatalf("CorrectFlow: %v", err)
	}
	var post *domain.FlowTransition
	for _, tr := range flow.Transitions {
		if tr.Name == "PostLines" {
			post = tr
		}
	}
	if post == nil {
		t.Fatal("PostLines transition not found")
	}
	if post.ForEachVar != "line" {
		t.Errorf("ForEachVar = %q, want line", post.ForEachVar)
	}
	if post.ForEachList != "<<invoice.lines>>" {
		t.Errorf("ForEachList = %q", post.ForEachList)
	}
	if post.True != "Done#1" || post.False != "Rollback#1" {
		t.Errorf("routing: true=%q false=%q", post.True, post.False)
	}
}

func TestBuildAST_ForEachRejectsTopLevelAction(t *testing.T) {
	src := `
module bad
workflow bad {
  start: S
  state S {
    action a/a.Extra()
    foreach x in <<items>> {
      action a/a.Do(v: x) as r
    }
    on success -> Done
    on fail -> Failed
  }
  state Done { end ok }
  state Failed { end fail }
}
`
	if _, _, err := wsl.ParseAll(src, "bad"); err == nil {
		t.Fatal("expected an error: foreach state cannot also have a top-level action")
	}
}
