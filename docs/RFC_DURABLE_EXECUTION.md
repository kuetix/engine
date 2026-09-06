# RFC: Durable workflow execution, `wait signal`, human-approval

Status: **draft for discussion.** No code. This is the Phase 6 design doc that
`ENGINE_EVOLUTION_PLAN.md` decision #6 scheduled for "after Phase 3" — Phases
0–5 are now landed, so this is the gate before any Phase 6 implementation.

Audience: engine maintainers. Read `ENGINE_EVOLUTION_PLAN.md` first.

---

## 1. What we're trying to make possible

Three capabilities, one underlying requirement.

| Capability | Example |
|---|---|
| **Wait for an external event** | order workflow pauses until payment webhook arrives |
| **Human approval step** | expense > $5k pauses until a manager approves in the UI |
| **Durable timers / `timeout`** | "cancel the reservation if not confirmed within 24h" |

All three need the same thing: **the engine must be able to suspend a running
workflow, persist everything needed to resume it, return control, and later
continue that exact run — possibly in a different process, hours or days later,
across a redeploy.**

Today the engine runs a workflow start-to-finish in one `Engine.Run()` call on
one goroutine, holding all state in memory (`WorkerSessionContext`, the context
map, in-flight `parallelGroup`s). There is no serialisation, no run identity, no
resume. That is the gap this RFC closes.

## 2. Non-goals / explicit scope limits

- **Not reimplementing Temporal.** No workflow-versioning-on-replay, no
  deterministic-replay VM, no built-in saga compensation engine. Those can be
  composed in WSL on top of the primitives here.
- **WSL text stays canonical** (constraint #3). Persistence stores *run state*,
  never a mutated copy of the workflow.
- **The transition wire protocol does not change** for the common case. The one
  place an ABI change is on the table — a context/deadline on the transition
  signature — is called out explicitly in §8 and is opt-in.
- **No distributed transactions.** A durable run is a state machine with a
  persisted cursor; steps are at-least-once (see §6).

## 3. The core abstraction: a `Run`

A **Run** is one execution of one workflow. It has:

| Field | Notes |
|---|---|
| `id` | ULID. The correlation key for signals, queries, the UI. |
| `workflow` | `org/name@version` + the engine version that validated it |
| `status` | `running` \| `suspended` \| `completed` \| `failed` \| `cancelled` |
| `cursor` | the current state name (`#`-hash form), i.e. where to resume |
| `context` | the serialised `values` map — result aliases, `let` bindings, `foreach`/`while` indices, constants snapshot |
| `history` | append-only list of completed steps: `{state, action, args, response, at}` — see §6 |
| `waiting_on` | when `suspended`: `{signal_name, timeout_at}` |
| `created_at` / `updated_at` / `finished_at` | |

Only JSON-serialisable values live in `context`. Transitions already receive
plain maps, so this is mostly already true; the audit in §9 confirms it.

### 3.1 The Store interface

```go
type RunStore interface {
    Create(ctx, run *Run) error
    Load(ctx, id string) (*Run, error)
    Save(ctx, run *Run) error                 // optimistic: fails on version mismatch
    ListWaiting(ctx, before time.Time) ([]*Run, error) // for the timer sweeper
    // signal delivery is a compare-and-set on (id, waiting_on.signal_name)
    DeliverSignal(ctx, id, name string, payload json.RawMessage) (delivered bool, err error)
}
```

Implementations: `memory` (tests, single-process dev), `redis` (default —
already a platform dependency), `postgres` (audit-heavy deployments). The engine
depends only on the interface; the concrete store is wired via DI / config
profile, exactly like the rest of the engine.

`Save` is optimistic-locked (a `version int` on the row) so two workers racing to
resume the same run can't both win.

## 4. Execution model change: step, persist, maybe suspend

`Engine.Run()` becomes a loop that can **yield**:

```
loop:
  step := engine.next(cursor)
  outcome := worker.ProcessState(step)     // unchanged
  switch outcome.kind:
    case advanced:   run.cursor = outcome.next; store.Save(run); continue
    case suspend:    run.status = suspended
                     run.waiting_on = outcome.wait
                     store.Save(run); return Suspended
    case terminal:   run.status = completed|failed
                     store.Save(run); return Done
```

Two modes, selected by config:

- **ephemeral** (today's behaviour, the default for CLI / `wsl_run` / anything
  without a `wait`): no store, no per-step Save, runs in one call. A workflow
  that contains a `wait` state fails validation in ephemeral mode ("this
  workflow needs a durable runner").
- **durable**: backed by a `RunStore`. Every state transition is a `Save`
  (cheap — small JSON). Hitting a `wait` state persists and returns.

`store.Save` on every step is the "checkpoint". It's what makes a crash
mid-workflow recoverable: on restart, a supervisor loads `running` runs whose
`updated_at` is stale and resumes them from `cursor`.

## 5. `wait signal` — the language surface

```wsl
state AwaitPayment {
  wait signal payment_received timeout: 24h
  on signal -> Fulfil
  on timeout -> Expire
}
```

- `wait signal <name>` — a new state kind (no action). The engine sets
  `run.waiting_on = {name, timeout_at?}`, persists, returns `Suspended`.
- `on signal -> S` — where to resume when the signal arrives. The signal payload
  is bound to an alias: `wait signal payment_received as payment` → `payment` in
  scope on resume.
- `on timeout -> S` — required if `timeout:` is given; the timer sweeper (§7)
  routes here.
- Grammar: `wait` is already a token (`TokWait`, used by parallel-join). This is
  a second `wait` form — `wait signal <name>` vs `wait <JoinName>`. Disambiguate
  on the token after `wait`.

**Hard constraint:** a `wait signal` state may **not** appear inside a
`parallel` state, a `foreach … parallel`, or a `foreach`/`while` body (v1).
Suspending mid-fan-out means serialising in-flight goroutines, which we're not
doing. `wsl_validate` enforces this. Waits live in the linear spine of a
workflow. (A later version can support "wait inside a branch" by persisting each
branch as its own child Run — noted in §11, not now.)

## 6. Step semantics on resume: at-least-once + the `history` log

When a run resumes from `cursor`, the engine re-enters `ProcessState` for that
state. Everything *before* `cursor` already ran and its effects are in
`context`. But two things need care:

1. **`let` bindings and `foreach`/`while` indices** — these are in `context`, so
   they survive. A resumed state does **not** re-run the `let`s of *earlier*
   states (it never re-enters them).
2. **The state at `cursor` itself, if it's a `wait`** — re-entering it just means
   "check for the signal / re-arm the wait". No action to re-run.
3. **A crash *between* a transition's action completing and `store.Save`** — the
   action ran, its response may not be persisted. On resume the engine re-runs
   that action. **This is at-least-once.** The mitigation is the same as
   `retry` (Phase 5): **transitions must be idempotent.** This is now a
   first-class contract, not just advice.

The `history` log records `{state, action, args-hash, response, at}` for every
completed step. Its purposes:

- **audit** — the UI shows the full path a run took, with timings.
- **optional memoisation** — a future "replay-safe" mode (§11) can skip
  re-running an action whose `history` entry matches; out of scope for v1 but
  the log is designed to support it.

For v1: `history` is append-only audit; correctness rests on idempotency.

## 7. Durable timers — and this is where Phase 5 `timeout` lands

A **timer sweeper** is a background service (one per deployment, leader-elected
or just idempotent):

```
every 10s:
  for run in store.ListWaiting(before = now):
    if store.DeliverSignal(run.id, "__timeout__", nil):
       enqueue run.id for resume
```

`timeout:` on a `wait signal` uses this. And the deferred **Phase 5 `timeout`
on an action** can be built the same way *only in durable mode*:

```wsl
state ChargeCard {
  timeout: 10s
  action payments.Charge(...) as charge
  on success -> Confirm
  on fail -> Failed
  on timeout -> Failed
}
```

In durable mode, `timeout` on an action means: run the action on a goroutine,
and if the deadline passes first, persist the run as `suspended` with a timeout
timer and **return** — the action goroutine keeps running but its result is
discarded when it finishes (the run has already moved on). Still leaky per-call,
but the *workflow* is unblocked and durable. This is strictly better than the
ephemeral-mode situation (where there's no answer at all) and does not need the
ABI change. Ephemeral-mode `timeout` stays unsupported.

## 8. The ABI question (transition context)

Cleanly cancelling an in-flight transition needs the transition to accept a
`context.Context` (or a deadline it polls). That is:

```go
// today
func (x *xT) ChargeCommand(command string, config, flags map[string]interface{}) domain.FlowStepResult
// proposed, additive
func (x *xT) ChargeCommand(ctx context.Context, command string, config, flags map[string]interface{}) domain.FlowStepResult
```

Per constraint #2 this is **the highest-severity kind of change** — it's the ABI
of every transition worker in every language. **This RFC does not propose making
it.** Options, in preference order:

1. **Cooperative deadline via the session context** (no ABI change) — the engine
   puts `deadline` in the `WorkerSessionContext`; long-running transitions that
   choose to can check it. Covers the well-behaved case. Ship this.
2. **Optional context as a detected second signature** — the engine reflects on
   the method; if the first param is `context.Context`, pass one. Old
   transitions keep working. This is a smaller change than it looks and worth a
   separate mini-RFC if #1 proves insufficient.
3. Full signature change — only with a major engine version and a migration.

## 9. Serialisation audit — what's in `context` today

Before building, confirm every value that flows into the workflow context is
JSON-round-trippable:

- ✅ result aliases — transitions return `interface{}` responses that are already
  marshalled at the API boundary
- ✅ `let` bindings — evaluator produces `nil/bool/int64/float64/string/[]/map`
- ✅ `foreach`/`while` indices — `int64`
- ✅ constants — parsed to the same scalar set
- ⚠️ `workflow.Flow`, `workflow.Worker`, `workflow.EngineInterface` — these
  live in the context map (`PrepareContext`) and are **not** serialisable. They
  must be *excluded* from `run.context` (they're re-created on resume, not
  restored). A `contextKeysToPersist` allowlist (or a `workflow.*` denylist)
  handles this.
- ⚠️ `*domain.Flow` stored as `Parent` — hierarchical runs (solution → feature →
  workflow) share one context. A suspended child needs its parent chain
  re-established on resume. v1: **only top-level workflows can contain `wait`**;
  hierarchical `wait` is §11.

## 10. API surface (the registry / `api/`)

- `POST /v1/runs` `{workflow, input}` → `{run_id, status}` — start a durable run
- `GET /v1/runs/{id}` → full Run (status, cursor, history, waiting_on)
- `POST /v1/runs/{id}/signals` `{name, payload}` → delivers a signal; 200 if the
  run was waiting on it, 409 if not
- `POST /v1/runs/{id}/cancel`
- `GET /v1/runs?workflow=&status=&waiting_on=` — list / filter (the UI's run
  inbox)

Human-approval is **not** a new primitive — it's:

1. a `wait signal approval_<id>` state, plus
2. a std transition `approvals/approvals.Request(...)` that (before the wait)
   creates an approval task row + notifies approvers, and
3. a UI page / `POST /v1/runs/{id}/signals {name: "approval_<id>", payload:
   {decision, by}}` when someone clicks approve/reject.

The engine only knows "wait for signal `approval_<id>`". Everything approval-
shaped is composed in WSL + a std package. This keeps the engine small.

## 11. Deliberately deferred (name them so they're not "forgotten fixes")

- **`wait` inside `parallel` / `foreach` / a branch** — needs child Runs per
  branch and a join that reconciles suspended children. Real design work.
- **Hierarchical `wait`** (solution/feature containing a wait) — parent-chain
  persistence.
- **Replay-safe / exactly-once step execution** — using `history` to memoise
  completed actions. Removes the idempotency requirement. Big.
- **Workflow migration for in-flight runs** — a run suspended on v1.2 of a
  workflow, resumed after v1.3 is published. v1: pin the run to its original
  version.
- **Backpressure / rate control on resume storms** — a deploy that resumes
  10k runs at once.

## 12. Proposed build order

| Step | Deliverable | Unblocks |
|---|---|---|
| 6a | `RunStore` interface + `memory` + `redis` impls; `Run` type; serialisation audit + allowlist | everything |
| 6b | durable `Engine.Run` (step→Save loop); ephemeral vs durable mode; `wsl_validate` rejects `wait` in ephemeral / inside parallel | crash recovery |
| 6c | `wait signal <name> [as alias]` state + `on signal` edge; signal delivery API; resume path | external-event workflows |
| 6d | timer sweeper; `timeout:` on `wait`; durable-mode `timeout:` on an action; cooperative deadline in session context | timeouts, expiry |
| 6e | approvals std package + UI signal endpoint; run-inbox API | human approval |

6a + 6b are the foundation and have value on their own (crash recovery for
long linear workflows) even before `wait` exists.

## 13. Open questions for the maintainer

1. Default store: Redis (lean, already there) vs Postgres (durable audit)?
   Recommendation: Redis default, Postgres opt-in.
2. Is at-least-once + idempotency acceptable for v1, or is replay-safe
   memoisation (§11) a hard requirement before shipping `wait`?
3. Cooperative deadline (§8 option 1) — enough for now, or do we want the
   detected-context-signature (option 2) mini-RFC in parallel?
4. `history` retention — keep forever (audit) or TTL completed runs?
5. Does `wait` need a max suspension lifetime (e.g. auto-cancel after 30d)?
