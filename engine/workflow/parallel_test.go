package workflow

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/internal/wsl"
)

// parallelTestTransition is a minimal service transition used to observe
// concurrent branch execution.
type parallelTestTransition struct {
	BaseServiceTransition
	calls    *int64
	failFrom int64 // branches with call ordinal > failFrom fail; 0 = never fail
}

func (p *parallelTestTransition) Do() domain.FlowStepResult {
	n := atomic.AddInt64(p.calls, 1)
	if p.failFrom > 0 && n > p.failFrom {
		return domain.FlowStepResult{Success: false, Error: errors.New("branch failed")}
	}
	return domain.FlowStepResult{Success: true, Response: fmt.Sprintf("done-%d", n)}
}

func newParallelTestWorker(t *testing.T, impl *parallelTestTransition) *workflowWorker {
	t.Helper()
	boot.MetaFunctionCache["tests"] = map[string]map[string]interfaces.FunctionMetadata{
		"parallel": {
			"Do": {
				Namespace:   "tests/parallel",
				Class:       "parallelTestTransition",
				Name:        "Do",
				NumIn:       0,
				NumOut:      1,
				ArgTypes:    []string{},
				ArgNames:    []string{},
				ReturnTypes: []string{"domain.FlowStepResult"},
			},
		},
	}
	t.Cleanup(func() { delete(boot.MetaFunctionCache, "tests") })

	mapping := &ServiceTransitionMapping{ServiceName: "tests", Name: "parallel", Impl: impl}
	ctx := map[string]interface{}{}
	return &workflowWorker{
		WorkflowContext:     NewWorkflowContext(&ctx),
		Transitions:         map[string]interfaces.ServiceTransitions{"tests/parallel": impl},
		TransitionsResolved: map[string]*ServiceTransitionMapping{"tests/parallel": mapping},
		Response:            &WorkerResponse{},
		LastResponse:        &WorkerResponse{},
	}
}

func parallelTestFlow(count int) *domain.Flow {
	return &domain.Flow{
		Name:       "startup",
		ConfigName: "startup",
		CurrentState: &domain.FlowState{
			State: "tests/parallel/Do#RegisterCommands",
		},
		CurrentTransition: &domain.FlowTransition{
			Name:          "RegisterCommands",
			To:            "RegisterCommands#1",
			ParallelCount: count,
			Response:      "Reg",
			True:          "BuildIndex#1",
		},
	}
}

func waitTestFlow() *domain.Flow {
	return &domain.Flow{
		Name:       "startup",
		ConfigName: "startup",
		CurrentState: &domain.FlowState{
			State: "Result#1",
		},
		CurrentTransition: &domain.FlowTransition{
			Name:     "Result",
			To:       "Result#1",
			WaitJoin: "RegisterCommands",
			True:     "Ready#1",
			False:    "Failed#1",
		},
	}
}

func TestParallelForkJoin_AllBranchesSucceed(t *testing.T) {
	var calls int64
	impl := &parallelTestTransition{calls: &calls}
	worker := newParallelTestWorker(t, impl)

	forkFlow := parallelTestFlow(6)
	forkSession := &WorkerSessionContext{
		Flow:            forkFlow,
		Worker:          worker,
		WorkflowContext: worker.WorkflowContext,
		Parser:          NewParser(),
	}

	ok, next := worker.processParallelFork(forkSession)
	if !ok {
		t.Fatalf("fork failed: %v", worker.GetError())
	}
	if next != "BuildIndex#1" {
		t.Errorf("fork next: got %q, want BuildIndex#1", next)
	}

	joinFlow := waitTestFlow()
	joinSession := &WorkerSessionContext{
		Flow:            joinFlow,
		Worker:          worker,
		WorkflowContext: worker.WorkflowContext,
		Parser:          NewParser(),
	}

	ok, next = worker.processParallelWait(joinSession)
	if !ok {
		t.Fatalf("wait failed: %v", worker.GetError())
	}
	if next != "Ready#1" {
		t.Errorf("wait next: got %q, want Ready#1", next)
	}

	if got := atomic.LoadInt64(&calls); got != 6 {
		t.Errorf("transition invocations: got %d, want 6", got)
	}

	alias := worker.WorkflowContext.Value("Reg")
	responses, ok := alias.([]interface{})
	if !ok {
		t.Fatalf("alias Reg: got %T, want []interface{}", alias)
	}
	if len(responses) != 6 {
		t.Errorf("alias Reg length: got %d, want 6", len(responses))
	}
	for i, r := range responses {
		if r == nil {
			t.Errorf("response[%d] is nil", i)
		}
	}
}

func TestParallelForkJoin_BranchFailureTakesErrorPath(t *testing.T) {
	var calls int64
	impl := &parallelTestTransition{calls: &calls, failFrom: 3} // branches 4..6 fail
	worker := newParallelTestWorker(t, impl)

	forkSession := &WorkerSessionContext{
		Flow:            parallelTestFlow(6),
		Worker:          worker,
		WorkflowContext: worker.WorkflowContext,
		Parser:          NewParser(),
	}
	if ok, _ := worker.processParallelFork(forkSession); !ok {
		t.Fatalf("fork failed: %v", worker.GetError())
	}

	joinSession := &WorkerSessionContext{
		Flow:            waitTestFlow(),
		Worker:          worker,
		WorkflowContext: worker.WorkflowContext,
		Parser:          NewParser(),
	}
	ok, next := worker.processParallelWait(joinSession)
	if !ok {
		t.Fatal("wait should route to the error path, not hard-fail")
	}
	if next != "Failed#1" {
		t.Errorf("wait next: got %q, want Failed#1", next)
	}
	if worker.GetError() == nil {
		t.Error("aggregated error should be set")
	}
	if got := atomic.LoadInt64(&calls); got != 6 {
		t.Errorf("all branches should run to completion: got %d, want 6", got)
	}
}

func TestParallelWait_WithoutForkFails(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})

	joinSession := &WorkerSessionContext{
		Flow:            waitTestFlow(),
		Worker:          worker,
		WorkflowContext: worker.WorkflowContext,
		Parser:          NewParser(),
	}
	ok, _ := worker.processParallelWait(joinSession)
	if ok {
		t.Fatal("wait without a started parallel group must fail")
	}
	if worker.GetError() == nil {
		t.Error("error should be set")
	}
}

// TestParallelSchema_EndToEnd verifies WSL text with parallel/wait states
// survives the full pipeline: parse -> schema -> domain.Flow.
func TestParallelSchema_EndToEnd(t *testing.T) {
	src := `
module startup

workflow startup {
  start: RegisterCommands

  parallel[count: 6] RegisterCommands {
    action commands/register.RegisterCommand() as Reg
    on success -> BuildIndex
  }

  state BuildIndex {
    action commands/index.Prepare() as Idx
    on success -> Result
  }

  wait Result {
    join RegisterCommands
    on success -> Ready
    on error -> Failed
  }

  state Ready {
    action commands/register.Finish()
    end ok
  }

  state Failed {
    end fail
  }
}
`
	_, graphs, err := wsl.ParseAll(src, "startup")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	schema := wslGraphToSchema(graphs["startup"])

	flow := &domain.Flow{Name: "startup", ConfigName: "startup", Properties: &domain.FlowOptions{}}
	if err := flow.FromMap(schema); err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	if err := (&Engine{}).CorrectFlow(flow); err != nil {
		t.Fatalf("CorrectFlow: %v", err)
	}

	var forkTr, waitTr *domain.FlowTransition
	for _, tr := range flow.Transitions {
		switch tr.Name {
		case "RegisterCommands":
			forkTr = tr
		case "Result":
			waitTr = tr
		}
	}
	if forkTr == nil || waitTr == nil {
		t.Fatalf("fork/wait transitions not found in flow: %+v", flow.Transitions)
	}
	if forkTr.ParallelCount != 6 {
		t.Errorf("fork ParallelCount: got %d, want 6", forkTr.ParallelCount)
	}
	if forkTr.Response != "Reg" {
		t.Errorf("fork Response alias: got %q, want Reg", forkTr.Response)
	}
	if waitTr.WaitJoin != "RegisterCommands" {
		t.Errorf("wait WaitJoin: got %q, want RegisterCommands", waitTr.WaitJoin)
	}
	if waitTr.True == "" || waitTr.False == "" {
		t.Errorf("wait transition should have both success and error paths: true=%q false=%q", waitTr.True, waitTr.False)
	}
}
