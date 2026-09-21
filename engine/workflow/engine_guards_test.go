package workflow

import (
	"strings"
	"testing"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
)

// guardCtx is a trivial WorkerContext backed by a map.
type guardCtx struct{ m map[string]interface{} }

func newGuardCtx() *guardCtx { return &guardCtx{m: map[string]interface{}{}} }

func (c *guardCtx) Context() *map[string]interface{}        { return &c.m }
func (c *guardCtx) SetContext(ctx *map[string]interface{})  { c.m = *ctx }
func (c *guardCtx) Value(name string) interface{}           { return c.m[name] }
func (c *guardCtx) SetValue(name string, value interface{}) { c.m[name] = value }

// guardWorker is a minimal Worker implementation for exercising Engine.Run()'s
// execution guards without the reflective transition machinery. route() decides
// where ProcessState sends the flow next; "" means "terminal / no custom next".
type guardWorker struct {
	ctx   *guardCtx
	resp  *WorkerResponse
	last  *WorkerResponse
	err   *issues.Issues
	debug bool
	steps int
	route func(step int) string
}

func newGuardWorker(route func(step int) string) *guardWorker {
	return &guardWorker{
		ctx:   newGuardCtx(),
		resp:  &WorkerResponse{},
		last:  &WorkerResponse{},
		route: route,
	}
}

func (g *guardWorker) Context() *map[string]interface{}                         { return g.ctx.Context() }
func (g *guardWorker) SetContext(c *map[string]interface{})                     { g.ctx.SetContext(c) }
func (g *guardWorker) Start(w EngineInterface) bool                             { return true }
func (g *guardWorker) InitResolvers(resolvers []string)                         {}
func (g *guardWorker) PrepareContext(w EngineInterface, flow *domain.Flow) bool { return true }
func (g *guardWorker) ProcessState(w EngineInterface, flow *domain.Flow) (bool, string) {
	g.steps++
	return true, g.route(g.steps)
}
func (g *guardWorker) Done(w EngineInterface) *WorkerResponse { return g.resp }
func (g *guardWorker) ProcessStateError(w EngineInterface, flow *domain.Flow, response *WorkerResponse) {
}
func (g *guardWorker) SetResponse(response any, statusCode ...int) { g.resp.Response = response }
func (g *guardWorker) GetResponse() any                            { return g.resp.Response }
func (g *guardWorker) SetLastResponse(response any, statusCode int) {
	g.last.Response = response
	g.last.StatusCode = statusCode
}
func (g *guardWorker) SetLastStatusCode(statusCode int)       { g.last.StatusCode = statusCode }
func (g *guardWorker) GetLastResponse() any                   { return g.last.Response }
func (g *guardWorker) GetWorkerResponse() *WorkerResponse     { return g.resp }
func (g *guardWorker) GetWorkerLastResponse() *WorkerResponse { return g.last }
func (g *guardWorker) HandleError(err interface{}, statusCode int) bool {
	if err != nil {
		if e, ok := err.(error); ok {
			g.SetError(issues.NewIssueFromError(e), statusCode)
			return false
		}
	}
	return true
}
func (g *guardWorker) SetError(err *issues.Issue, statusCode ...int) bool {
	if g.err == nil {
		g.err = &issues.Issues{}
	}
	g.err.Another(err)
	return false
}
func (g *guardWorker) SetErrors(errs *issues.Issues, statusCode ...int) bool {
	g.err = errs
	return false
}
func (g *guardWorker) MergeIssues(errs *issues.Issues, statusCode ...int) bool {
	if g.err == nil {
		g.err = errs
	} else {
		g.err.More(errs.Errors()...)
	}
	return false
}
func (g *guardWorker) GetError() *issues.Issues          { return g.err }
func (g *guardWorker) SetStatusCode(statusCode int)      { g.resp.StatusCode = statusCode }
func (g *guardWorker) GetStatusCode() int                { return g.resp.StatusCode }
func (g *guardWorker) SetDebug(debug bool)               { g.debug = debug }
func (g *guardWorker) IsDebug() bool                     { return g.debug }
func (g *guardWorker) GetWorkflowContext() WorkerContext { return g.ctx }
func (g *guardWorker) CleanErrors() bool                 { g.err = nil; return true }

func selfLoopFlow() *domain.Flow {
	return &domain.Flow{
		Name:       "loop",
		ConfigName: "loop",
		States: []*domain.FlowState{
			{State: "A#1", Type: domain.StateNormal},
			{State: "Done#1", Type: domain.StateFinal},
		},
		Transitions: []*domain.FlowTransition{
			{Name: "A", To: "A#1", From: []string{"_", "A#1"}, True: "A#1"},
		},
	}
}

func twoStateCycleFlow() *domain.Flow {
	return &domain.Flow{
		Name:       "cycle",
		ConfigName: "cycle",
		States: []*domain.FlowState{
			{State: "A#1", Type: domain.StateNormal},
			{State: "B#1", Type: domain.StateNormal},
		},
		Transitions: []*domain.FlowTransition{
			{Name: "A", To: "A#1", From: []string{"_", "B#1"}, True: "B#1"},
			{Name: "B", To: "B#1", From: []string{"A#1"}, True: "A#1"},
		},
	}
}

func TestRun_StateVisitCapAbortsSelfLoop(t *testing.T) {
	e := &Engine{Name: "test", WorkflowName: "loop", Flow: selfLoopFlow()}
	gw := newGuardWorker(func(int) string { return "A" })
	e.Worker = gw

	if e.Run() {
		t.Fatal("Run() returned true for a non-terminating workflow, want false")
	}
	if gw.GetError() == nil {
		t.Fatal("expected an error after the visit cap tripped, got none")
	}
	msg := gw.GetError().Error()
	if !strings.Contains(msg, "infinite loop") || !strings.Contains(msg, "A#1") {
		t.Fatalf("error %q should mention 'infinite loop' and the offending state", msg)
	}
	if gw.steps > MaxStateVisits+2 {
		t.Fatalf("loop ran %d steps, expected abort near the cap of %d", gw.steps, MaxStateVisits)
	}
}

func TestRun_StateVisitCap_RespectsConfiguredLimit(t *testing.T) {
	orig := MaxStateVisits
	MaxStateVisits = 5
	defer func() { MaxStateVisits = orig }()

	e := &Engine{Name: "test", WorkflowName: "loop", Flow: selfLoopFlow()}
	gw := newGuardWorker(func(int) string { return "A" })
	e.Worker = gw

	e.Run()

	if gw.steps > MaxStateVisits+2 {
		t.Fatalf("loop ran %d steps, expected abort near the cap of %d", gw.steps, MaxStateVisits)
	}
}

func TestRun_StepBudgetAbortsLongCycle(t *testing.T) {
	origVisits, origSteps := MaxStateVisits, MaxSteps
	MaxStateVisits = 1 << 30 // disable the per-state cap
	MaxSteps = 50
	defer func() { MaxStateVisits, MaxSteps = origVisits, origSteps }()

	e := &Engine{Name: "test", WorkflowName: "cycle", Flow: twoStateCycleFlow()}
	// alternate A / B so no single state hits the (disabled) per-state cap
	gw := newGuardWorker(func(step int) string {
		if step%2 == 0 {
			return "A"
		}
		return "B"
	})
	e.Worker = gw

	if e.Run() {
		t.Fatal("Run() returned true for a non-terminating cycle, want false")
	}
	if gw.GetError() == nil {
		t.Fatal("expected an error after the step budget tripped, got none")
	}
	if msg := gw.GetError().Error(); !strings.Contains(msg, "step budget") {
		t.Fatalf("error %q should mention the step budget", msg)
	}
}

func TestRun_TerminatingWorkflowUnaffectedByGuards(t *testing.T) {
	flow := &domain.Flow{
		Name:       "ok",
		ConfigName: "ok",
		States: []*domain.FlowState{
			{State: "A#1", Type: domain.StateNormal},
			{State: "Done#1", Type: domain.StateFinal},
		},
		Transitions: []*domain.FlowTransition{
			{Name: "A", To: "A#1", From: []string{"_"}, True: "Done#1"},
			{Name: "Done", To: "Done#1", From: []string{"A#1"}, Type: domain.StateFinal, FinalKind: "ok"},
		},
	}
	e := &Engine{Name: "test", WorkflowName: "ok", Flow: flow}
	gw := newGuardWorker(func(int) string { return "" })
	e.Worker = gw

	if !e.Run() {
		t.Fatalf("Run() returned false for a terminating workflow: %v", gw.GetError())
	}
	if gw.GetError() != nil {
		t.Fatalf("terminating workflow should not error: %v", gw.GetError())
	}
}
