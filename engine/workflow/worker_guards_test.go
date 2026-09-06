package workflow

import (
	"testing"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/internal/wsl"
)

// guardRoutingFlow builds a one-state flow whose transition carries ordered
// `on success when` guards plus an unguarded fallback (True).
func guardRoutingFlow() *domain.Flow {
	return &domain.Flow{
		Name:       "gate",
		ConfigName: "gate",
		CurrentState: &domain.FlowState{
			State: "tests/parallel/Do#Check",
		},
		CurrentTransition: &domain.FlowTransition{
			Name: "Check",
			To:   "Check#1",
			Guards: []domain.FlowGuard{
				{When: `version == "1.0.0"`, To: "VersionOne#1"},
				{When: `version == "2.0.0"`, To: "VersionTwo#1"},
			},
			True: "DefaultVersion#1",
		},
		States: []*domain.FlowState{
			{State: "Check#1", Type: domain.StateNormal},
			{State: "VersionOne#1", Type: domain.StateFinal},
			{State: "VersionTwo#1", Type: domain.StateFinal},
			{State: "DefaultVersion#1", Type: domain.StateFinal},
		},
	}
}

func TestProcessState_GuardRouting(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		{"1.0.0", "VersionOne#1"},
		{"2.0.0", "VersionTwo#1"},
		{"3.0.0", "DefaultVersion#1"}, // no guard matches -> fallback
	}
	for _, c := range cases {
		var calls int64
		impl := &parallelTestTransition{calls: &calls}
		worker := newParallelTestWorker(t, impl)
		worker.WorkflowContext.SetValue("version", c.version)

		flow := guardRoutingFlow()
		ok, next := worker.ProcessState(nil, flow)
		if !ok {
			t.Fatalf("version %s: ProcessState failed: %v", c.version, worker.GetError())
		}
		if next != c.want {
			t.Errorf("version %s: routed to %q, want %q", c.version, next, c.want)
		}
	}
}

func TestProcessState_GuardRouting_FirstMatchWins(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})
	worker.WorkflowContext.SetValue("n", int64(5))

	flow := guardRoutingFlow()
	flow.CurrentTransition.Guards = []domain.FlowGuard{
		{When: `n > 0`, To: "First#1"},
		{When: `n > 3`, To: "Second#1"},
	}
	flow.States = append(flow.States,
		&domain.FlowState{State: "First#1", Type: domain.StateFinal},
		&domain.FlowState{State: "Second#1", Type: domain.StateFinal},
	)

	_, next := worker.ProcessState(nil, flow)
	if next != "First#1" {
		t.Errorf("routed to %q, want First#1 (first matching guard wins)", next)
	}
}

func TestProcessState_GuardRouting_NoFallbackFailsClosed(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})
	worker.WorkflowContext.SetValue("version", "9.9.9")

	flow := guardRoutingFlow()
	flow.CurrentTransition.True = "" // remove the fallback

	ok, _ := worker.ProcessState(nil, flow)
	if ok {
		t.Error("with no matching guard and no fallback, the state should not succeed")
	}
}

func TestProcessState_LetBindings(t *testing.T) {
	var calls int64
	worker := newParallelTestWorker(t, &parallelTestTransition{calls: &calls})
	worker.WorkflowContext.SetValue("rate", int64(3))

	flow := &domain.Flow{
		Name:       "lets",
		ConfigName: "lets",
		CurrentState: &domain.FlowState{
			State: "tests/parallel/Do#S",
		},
		CurrentTransition: &domain.FlowTransition{
			Name: "S",
			To:   "S#1",
			Lets: []domain.FlowLet{
				{Name: "base", Expr: "10"},
				{Name: "total", Expr: "base * rate"}, // references an earlier let + context
			},
			Guards: []domain.FlowGuard{
				{When: "total >= 30", To: "Big#1"},
			},
			True: "Small#1",
		},
		States: []*domain.FlowState{
			{State: "S#1", Type: domain.StateNormal},
			{State: "Big#1", Type: domain.StateFinal},
			{State: "Small#1", Type: domain.StateFinal},
		},
	}

	ok, next := worker.ProcessState(nil, flow)
	if !ok {
		t.Fatalf("ProcessState failed: %v", worker.GetError())
	}
	wsc := &WorkerSessionContext{WorkflowContext: worker.WorkflowContext, Worker: worker, Flow: flow, Parser: NewParser()}
	if got := wsc.Property("base"); got != int64(10) {
		t.Errorf("let base = %#v, want 10", got)
	}
	if got := wsc.Property("total"); got != int64(30) {
		t.Errorf("let total = %#v, want 30 (base * rate)", got)
	}
	if next != "Big#1" {
		t.Errorf("routed to %q, want Big#1 (guard total >= 30)", next)
	}
}

// End-to-end: WSL text with multiple `on success when` survives parse -> schema
// -> domain.Flow -> CorrectFlow as an ordered guard list.
func TestGuards_SchemaPipeline(t *testing.T) {
	src := `
module versioned

const { version: "2.0.0" }

workflow versioned {
  start: CheckVersion

  state CheckVersion {
    action app/app.Check() as vc
    on success when $constants.version == "1.0.0" -> VersionOne
    on success when $constants.version == "2.0.0" -> VersionTwo
    on success -> DefaultVersion
    on fail -> Failed
  }

  state VersionOne { end ok }
  state VersionTwo { end ok }
  state DefaultVersion { end ok }
  state Failed { end fail }
}
`
	_, graphs, err := wsl.ParseAll(src, "versioned")
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	schema := wslGraphToSchema(graphs["versioned"])
	flow := &domain.Flow{Name: "versioned", ConfigName: "versioned", Properties: &domain.FlowOptions{}}
	if err := flow.FromMap(schema); err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	if err := (&Engine{}).CorrectFlow(flow); err != nil {
		t.Fatalf("CorrectFlow: %v", err)
	}

	var check *domain.FlowTransition
	for _, tr := range flow.Transitions {
		if tr.Name == "CheckVersion" {
			check = tr
		}
	}
	if check == nil {
		t.Fatalf("CheckVersion transition not found")
	}
	if len(check.Guards) != 2 {
		t.Fatalf("expected 2 guards, got %d: %+v", len(check.Guards), check.Guards)
	}
	if check.Guards[0].When != `$constants.version == "1.0.0"` || check.Guards[0].To != "VersionOne#1" {
		t.Errorf("guard[0] = %+v", check.Guards[0])
	}
	if check.Guards[1].To != "VersionTwo#1" {
		t.Errorf("guard[1].To = %q, want VersionTwo#1", check.Guards[1].To)
	}
	if check.True != "DefaultVersion#1" {
		t.Errorf("fallback True = %q, want DefaultVersion#1", check.True)
	}
	if check.False != "Failed#1" {
		t.Errorf("False = %q, want Failed#1", check.False)
	}
}
