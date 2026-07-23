package wsl

import (
	"strings"
	"testing"
)

const parallelSrc = `
module startup

workflow startup {
  start: RegisterCommands

  parallel[count: 6] RegisterCommands {
    action commands/register.RegisterCommand(shard: $branch.index) as Reg
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
    action response/ResponseLastSuccess(registered: $Reg)
    end ok
  }

  state Failed {
    end fail
  }
}
`

func TestParallelWait_Parse(t *testing.T) {
	mod, graphs, err := ParseAll(parallelSrc, "startup")
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	wf := mod.Workflows[0]

	reg, ok := wf.States["RegisterCommands"]
	if !ok {
		t.Fatal("RegisterCommands state not found")
	}
	if !reg.Parallel {
		t.Error("RegisterCommands should be parallel")
	}
	if reg.ParallelCount != 6 {
		t.Errorf("ParallelCount: got %d, want 6", reg.ParallelCount)
	}
	if reg.Action == nil || reg.Action.Name != "RegisterCommand" {
		t.Errorf("parallel state action not parsed: %+v", reg.Action)
	}
	if reg.Action.As != "Reg" {
		t.Errorf("alias: got %q, want Reg", reg.Action.As)
	}

	res, ok := wf.States["Result"]
	if !ok {
		t.Fatal("Result state not found")
	}
	if !res.Wait {
		t.Error("Result should be a wait state")
	}
	if res.JoinTarget != "RegisterCommands" {
		t.Errorf("JoinTarget: got %q, want RegisterCommands", res.JoinTarget)
	}
	if len(res.Transitions) != 2 {
		t.Errorf("wait transitions: got %d, want 2", len(res.Transitions))
	}

	g := graphs["startup"]
	if g == nil {
		t.Fatal("graph not built")
	}
	if n := g.Nodes["RegisterCommands"]; !n.Parallel || n.ParallelCount != 6 {
		t.Errorf("graph node parallel flags not propagated: %+v", n)
	}
	if n := g.Nodes["Result"]; !n.Wait || n.JoinTarget != "RegisterCommands" {
		t.Errorf("graph node wait flags not propagated: %+v", n)
	}
}

func TestParallelWait_Validation(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name: "parallel without wait",
			src: `
module m
workflow w {
  start: P
  parallel[count: 2] P {
    action svc/x.Do()
    on success -> Done
  }
  state Done { end ok }
}`,
			wantErr: "no matching 'wait'",
		},
		{
			name: "parallel without count",
			src: `
module m
workflow w {
  start: P
  parallel P {
    action svc/x.Do()
    on success -> W
  }
  wait W {
    join P
    on success -> Done
  }
  state Done { end ok }
}`,
			wantErr: "'count' attribute",
		},
		{
			name: "wait without join",
			src: `
module m
workflow w {
  start: S
  state S {
    action svc/x.Do()
    on success -> W
  }
  wait W {
    on success -> Done
  }
  state Done { end ok }
}`,
			wantErr: "must declare 'join",
		},
		{
			name: "wait joins non-parallel state",
			src: `
module m
workflow w {
  start: S
  state S {
    action svc/x.Do()
    on success -> W
  }
  wait W {
    join S
    on success -> Done
  }
  state Done { end ok }
}`,
			wantErr: "not a parallel state",
		},
		{
			name: "count must be positive",
			src: `
module m
workflow w {
  start: P
  parallel[count: 0] P {
    action svc/x.Do()
    on success -> W
  }
  wait W {
    join P
    on success -> Done
  }
  state Done { end ok }
}`,
			wantErr: "must be >= 1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ParseAll(tc.src, "m")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
