# Engine Evolution Plan — expressions, control flow, concurrency, tests

Status: **in progress** — Phase 0 implemented on branch `engine-evolution`.
Per `CLAUDE.md` ("Known open gaps — do not silently fix; discuss design first")
this document holds the agreed design; phases land one at a time with tests green.

Audience: engine maintainers and contributors.

### Decisions taken (2026-09-06)

| # | Question | Decision |
|---|---|---|
| 1 | `parallel` keyword | **deferred to Phase 4** — no rename yet; revisit with the generalisation |
| 2 | `<<token>>` vs `${…}` | **keep both indefinitely**, no deprecation |
| 3 | Boolean strictness in `&&`/`||`/`when`/`if` | **truthiness allowed** (JS/Python style): non-empty string, non-zero number, non-null, non-empty list/map are truthy; `null`, `false`, `0`, `""`, `[]`, `{}` are falsy |
| 4 | `foreach` binding syntax | TBD at Phase 3 |
| 5 | step-budget defaults | package vars `MaxSteps=10000`, `MaxStateVisits=1000`; a config profile may override |
| 6 | Phase 6 durable-execution RFC | **write it after Phase 3** |

---

## 1. Where the engine actually is today (verified against source)

The claims below are checked against the current tree, not from memory.

### 1.1 There is no expression evaluator

- The lexer (`internal/wsl/lexer.go`) emits only `=`, `>`, `<`, `!` as
  operator-ish tokens. `==` is lexed as **two** `=` tokens; the `when` parser
  test literally asserts the raw string comes back as `` `$constants.version = =
  "1.0.0"` `` (`internal/wsl/when_test.go:77`). `+ - * / %` are treated as
  identifier characters (`lexer.go:105`), so `$base * 2` is one token.
- `when` / `if` expressions are stored as an opaque `Expr{Raw string}`
  (`internal/wsl/ast.go:91`, `ir.go:45`) and carried into the flow as
  `on_success_when` / `if` strings (`engine/workflow/wsl_integration.go:105-115`).
- At runtime the worker calls `Parser.ParseTemplate(conditionProp)` and then
  checks **`if condition == "false"`** (`engine/workflow/worker.go:258-266` for
  `if`, `484-511` for `on success when`). `ParseTemplate`
  (`engine/workflow/parser.go`) only does `<<token>>` substitution plus the `??`
  and `||` fallback operators. It never evaluates `==`, `>`, `<`, `&&`.
- Consequence: `on success when <<result.ok>> == true` resolves to the string
  `"true == true"`, which is `!= "false"`, so the success path is taken. But
  `<<result.ok>> == true` when `result.ok` is false resolves to `"false == true"`
  — also `!= "false"` — so the **same branch is taken**. The guard does not gate.
- The existing tests (`engine/workflow/worker_on_success_when_test.go`) only
  assert that the `OnSuccessWhen` **string field is stored**. None asserts that
  execution routes to the correct state. The behaviour is untested.

**This is the keystone gap.** Arithmetic, string building, real conditionals,
`while`, and loop guards all reduce to "evaluate a typed expression against the
workflow context". Build that once, correctly, and most of the rest is wiring.

### 1.2 No arithmetic, no string concatenation, no computed values

Same root cause. `const` values are parsed as literals with type coercion
(`engine/workflow/literals.go`), not expressions. Action args are literals or
`<<token>>` references, not expressions.

### 1.3 "Local variables" = result aliases only

`action X(...) as foo` binds the step result to `foo` in the context. SWSL adds
`<-` error binding. There is no `let x = <expr>`. There is no way to name an
intermediate value that isn't a transition's return.

### 1.4 No loop / iteration construct

- There is **no `foreach` and no `while`** in the grammar.
- State **revisits do work** mechanically: `ProcessState` returns the next state
  name and `Engine.Run` (`engine/workflow/engine.go:847-901`) loops
  `for w.can(customNextStepName)`. A transition that returns `Next: "SameState"`
  will re-run it.
- **But**: the run loop has **no step budget and no per-state visit cap**. A
  workflow that revisits without a terminating condition runs forever. `LimitTrace`
  (`engine.go:875`) only truncates the trace ring buffer; it does not stop the loop.
- And there is no runtime re-evaluation of `when`, so you cannot express the exit
  condition in WSL — the loop and its termination check must both live inside one
  Go transition today (the `packages/ai` `agent.Run` pattern).

### 1.5 `parallel` is homogeneous fan-out, not general parallelism

- `parallel[count: N] Name { action ... }` runs **the same action N times**
  concurrently, each branch seeing `branch.index` / `branch.count`
  (`engine/workflow/parallel.go:73-107`). A matching `wait Name { join ... }`
  state blocks on all branches and aggregates
  (`parallel.go:168-241`), binding the alias to an ordered `[]response`.
- Branch failure → aggregated error on the wait state's `on fail` path; all
  branches always run to completion (no cancellation).
- There is **no** heterogeneous fork (`run A and B and C concurrently`), and **no**
  data fan-out (`for each item in list, concurrently`).
- Grammar: `TokParallel` / `TokWait` / `TokJoin` (`internal/wsl/tokens.go:32-34`,
  `lexer.go:313-318`), parsed in `internal/wsl/parser.go:400-508`. SWSL sugar:
  `action()[count: N] as Alias -> next` lowers to fork + synthetic `_join` wait
  (`internal/wsl/simplified_wsl_parallel_test.go`).

### 1.6 I/O primitives (crypto / HTTP / file / Redis) — working as designed

The downstream "10 packages exist because the engine can't do X" observation is
**mostly correct-by-design**, not a bug:

| What the package does | Right home |
|---|---|
| Call an external HTTP API | Go transition (`std-http` exists) |
| Run a Redis Lua script / speak a binary protocol | Go transition |
| Crypto (AES/RSA/ed25519…) | Go transition (`cryptor` lib exists) |
| Wrap the HTTP server that runs workflows | Boot code, not WSL |
| **Loop over a Redis result set** | **`foreach` (this plan)** |
| **Time / priority arithmetic** | **expression evaluator (this plan)** |
| **Branch on a fetched value** | **real `when` gating (this plan)** |

So: **do not put HTTP/crypto/file into the language.** Two language features —
expressions and `foreach` — unblock the parts that genuinely need language
support; the rest stays in Go transitions permanently and correctly. The
follow-up is a *transition-library* audit (section 6), not an engine change.

---

## 2. The `parallel` → `concurrency` rename question

**Recommendation: keep `parallel` as the user-facing keyword. Do not rename.**

Reasoning:

- The audience for WSL text is business-logic authors, not systems programmers.
  "These steps run in parallel" communicates the intent (they happen at the same
  time, logically) better than "concurrency". The Pike distinction
  (concurrency = composition of independent processes; parallelism = simultaneous
  execution) is real but is an *implementation* detail of the Go engine, not a
  property the workflow author is choosing.
- Renaming is a **WSL text-format change**. Every published workflow that uses
  `parallel` / `wait` / `join` breaks, or needs an alias + deprecation window.
  That cost buys only a word change — it does **not** fix the actual gap, which
  is that there's only one shape of concurrency.
- If we *do* rename, do it **once, now**, while registry adoption is near zero,
  with `concurrency` as the canonical spelling and `parallel` as a parser alias
  that emits a deprecation diagnostic. Pick one.

**The real fix** is to make `parallel` an umbrella over three shapes (section
5.4):

1. `parallel[count: N]` — replicated fan-out (exists, keep).
2. `parallel { branch {…} branch {…} }` — heterogeneous fork/join (new).
3. `foreach x in <list> parallel[limit: K] -> …` — bounded data fan-out (new),
   which generalises and can eventually deprecate shape 1.

Decision needed from maintainer: **keep `parallel` / rename to `concurrency` /
rename to something else.** Everything else in section 5.4 is independent of the
spelling.

---

## 3. Design principles for all of this

1. **One evaluator.** `when`, `if`, `assert`, arithmetic, string interpolation,
   `let`, loop guards, `retry ... on:` — all parse to the same `Expr` AST and run
   through the same typed evaluator. No second expression path.
2. **Pure and deterministic.** The evaluator has no side effects and cannot call
   service transitions. It may call a **fixed whitelist of pure builtins**
   (`len`, `lower`, `upper`, `contains`, `startsWith`, `endsWith`, `int`,
   `float`, `string`, `bool`, `default`, `has`). I/O stays in transitions.
3. **The wire protocol does not change.** Transitions still receive resolved
   `config` / `flags` maps. Expressions are evaluated **engine-side** before the
   call. This keeps every change in this plan *below* the ABI severity bar in
   `CLAUDE.md` §"architectural constraints" #2 — call that out in each phase.
4. **Text format changes are additive.** New syntax; old `.wsl` still parses. A
   workflow using new syntax simply requires a newer engine, which the
   "validated by engine vX.Y" registry badge already communicates.
5. **Validation gets stricter, never looser.** Every new construct adds
   `wsl_validate` checks (unknown identifiers, type mismatches, missing loop
   guards, unbalanced fork/join). The registry's publish-time guarantee holds.
6. **Bounded by construction.** Every loop has a static or explicit upper bound.
   The engine also gets a global step budget as a backstop (Phase 0).
7. **Single-assignment bindings.** `let` names are immutable within a workflow
   run. This keeps flows statically analysable and sidesteps the "state is not
   re-evaluated on revisit" problem.

---

## 4. Phase 0 — safety net (no syntax change) — **DONE**

Small, high-value, zero ABI/text-format impact. Landed on `engine-evolution`.

| Item | Status | Change |
|---|---|---|
| Global step budget | ✅ | `Engine.Run` counts state transitions; aborts at `MaxSteps` (var, default 10000) with a trace in the error. `engine/workflow/engine.go` |
| Per-state visit cap | ✅ | Tracks visits per destination state; aborts at `MaxStateVisits` (var, default 1000) with a "possible infinite loop: state X" error + trace. |
| `==` / `!=` / `>=` / `<=` / `&&` / `||` as single tokens | ✅ | New `TokEqEq TokNeq TokGte TokLte TokAndAnd TokOrOr` in `tokens.go`; two-char match in `lexer.go` before the single-char switch (after `->` / `<-`). `=` still lexes alone for attributes. `Expr.Raw` now reads `a == b` instead of `a = = b`. |
| Stale `= =` test expectations | ✅ | Updated in `internal/wsl/when_test.go`, `internal/wsl/when_examples_file_test.go`, `engine/workflow/wsl_attributes_test.go`. |
| `wsl_validate` deprecation diagnostic on `when` comparisons | **descoped** | The `internal/wsl` package has no diagnostics framework (only hard errors). Building one is Phase 1 work, and Phase 1 makes `when` actually evaluate — so a stopgap warning has little value. Rolled into Phase 1. |

Tests added: `internal/wsl/lexer_test.go` `TestLexer_ComparisonAndLogicalOperators`
(15 cases incl. operator/arrow disambiguation, attribute `=`, no-space forms);
`engine/workflow/engine_guards_test.go` — self-loop trips the visit cap;
lowered-cap config is honoured; a long A/B cycle trips the step budget with the
per-state cap disabled; a terminating workflow is unaffected.

Baseline note: `internal/wsl` has 10 pre-existing failing tests
(`example_file_test.go` etc.) that read fixtures from a `runtime/workflows/`
tree absent from this checkout. Not caused by, and not fixed by, this work.

---

## 5. Phased feature design

### Phase 1 — expression evaluator + real `when` / `if` + arithmetic + string interpolation — **DONE (core)**

Landed on `engine-evolution`. What shipped vs. the original design below:

| Design item | Shipped | Notes |
|---|---|---|
| Expression grammar + precedence-climbing parser | ✅ `internal/wsl/expr.go` | **standalone lexer** (not the WSL lexer) so `-` `/` are operators inside expressions without disturbing qualified action names. Operates on the raw condition string captured by the WSL parser. |
| AST node tree | ✅ `LitExpr PathExpr UnaryExpr BinaryExpr CallExpr ListExpr MapExpr IndexExpr InterpExpr` | in `internal/wsl`, exported. `wsl.Expr{Raw}` **left untouched** — the tree is parsed lazily at the worker, not stored in AST/IR/flow-JSON. Less surface area; build-time expression validation deferred (see gaps). |
| Evaluator | ✅ `engine/workflow/expr_eval.go` | `EvalExpr(node, ExprScope)`. Values: `nil bool int64 float64 string []interface{} map[string]interface{}`. |
| Arithmetic `+ - * / %` | ✅ | int/int→int; `/` promotes to float on non-even division; any float operand→float; `÷0` and `%0` → error; bool/null/string operands → error. |
| String `+` concat, `${…}` interpolation | ✅ | `"a"+"b"`; `"hi ${user.name}, ${n+1} left"`. `"a"+1` errors. |
| Comparisons | ✅ | `== !=` deep-equal, int↔float cross-type; `"1"==1` is `false` (not an error); `< <= > >=` numeric or string, mixed → error. |
| `&&` `\|\|` `!` `??` with truthiness (decision #3) | ✅ | JS-style: `&&`/`\|\|` return an operand; `!`/`when`/`if` apply `Truthy()`. Falsy set: `null false 0 0.0 "" [] {}`. |
| Builtins whitelist | ✅ | `len lower upper contains startsWith endsWith int float string bool default has`. No transition calls. |
| `when` / `if` wiring in the worker | ✅ `engine/workflow/worker.go` | `evaluateTransitionCondition()` — parse → resolve via existing `WorkerSessionContext.GetProperty` → eval → truthiness. **Falls back** to the legacy `ParseTemplate` path when the string is not a parseable expression. Eval errors surface on the worker and fail the guard closed. |
| Parse-tree cache | ✅ | `sync.Map`, concurrency-safe (parallel branches evaluate conditions concurrently — race-tested). |
| Multiple `on success when` ordering | ✅ **Phase 1.1** | see below |
| Build-time expression validation | ✅ **Phase 1.1** | see below |
| Expressions in action args / `const` | Phase 2 (see below) | |

**Behaviour change (intended, flag for migration):** conditions that previously
"passed" only because the evaluator never actually evaluated them will now route
by their real value. e.g. `if $enabled == true` with `$enabled` unset used to
fall through as truthy; it now evaluates `null == true` → false → takes `else`.
This is the keystone fix, but any existing `.wsl` relying on the old
no-op behaviour will route differently. Legacy non-expression condition strings
are unaffected (template fallback).

Tests: `internal/wsl/expr_test.go` (parse: 30 valid, 11 invalid, AST shape),
`internal/wsl/expr_fuzz_test.go` (1M+ execs, no panic),
`engine/workflow/expr_eval_test.go` (operator tables, coercion, truthiness,
short-circuit, precedence, `??`, path resolution incl. nested traversal,
builtins + errors), `engine/workflow/worker_when_gating_test.go` (the section
7.4 matrix through `evaluateTransitionCondition`: `result.ok == false` now gates,
`&&` chains, missing paths, eval-error fails closed, `<<token>>` still works).

---

### Phase 1.1 — multiple guards + build-time validation — **DONE**

Landed on `engine-evolution`.

**Multiple `on success when` (§7.4 rows 4, 5, 9).** A state may now have several
`on success when <expr> -> <to>` edges plus one unguarded `on success -> <to>`.

- `domain.FlowTransition` gains `Guards []FlowGuard` (`{When, To}`), ordered.
- `wsl_integration.go` `edgeMap` collects **all** guarded success edges in source
  order into `guards`; the unguarded `on success` becomes `True` (the fallback).
  Back-compat: a lone guard with no unguarded fallback still lowers to the old
  `on_success_when` so hand-written JSON flows are unaffected.
- `CorrectFlow` `#`-normalises each guard target like `True`/`False`.
- Worker (`worker.go`, the `isDone` block): evaluates `Guards` top-to-bottom,
  **first truthy wins** → its `To`. None match → `True`, else `False`, else fail
  closed. `Guards` takes precedence over `OnSuccessWhen`.
- Semantics: `wsl_validate` still needs to *enforce* "unguarded `on success` must
  be last" — not done yet (the lowering already treats it as the fallback
  regardless of position; a lint is the remaining piece).

Tests: `engine/workflow/worker_guards_test.go` — `ProcessState` routes by guard
value (v1→VersionOne, v2→VersionTwo, v3→fallback); first-match-wins with
overlapping guards; no-match + no-fallback fails closed; full WSL-text →
schema → `FromMap` → `CorrectFlow` pipeline produces the ordered `Guards` with
normalised targets. Updated `wsl_on_success_when_test.go` /
`wsl_attributes_test.go` which asserted the old first-guard-only collapse.

**Build-time expression validation.** `internal/wsl` `Expr` gains a `Tree
ExprNode` field. `build_ast.go` now parses every `when` / `if` expression via
`ParseExpr` as it builds the AST and returns a `SemanticError` on failure — so
an unparseable condition fails `ParseAll` / `wsl_validate` / `kue publish`. The
tree is carried through AST→IR (available to Phase 2 and to arg-type-checking
later).

Tests: `internal/wsl/expr_test.go` `TestBuildAST_RejectsMalformedWhen` /
`TestBuildAST_AcceptsValidWhen`.

Still open (small, not blocking): identifier-level validation (unknown constant
/ alias references flagged at build time) — needs the scope known at build time;
and the "unguarded success must be last" lint.

---

<details>
<summary>Original Phase 1 design (for reference)</summary>

**Grammar.** Introduce an expression sublanguage. Precedence (low→high):
`||` → `&&` → `== != ` → `< <= > >=` → `+ -` → `* / %` → unary `! -` →
primary (`literal | ident.path | ( expr ) | builtin(args)`).

- `ident.path` resolves against the workflow context: `constants.*`, result
  aliases, `let` names, `branch.index`, `input.*`.
- Literals: `int64`, `float64`, `"string"` (with `${expr}` interpolation),
  `true`/`false`, `null`, `[a, b]`, `{k: v}`.
- Builtins: the whitelist from principle #2. No transition calls.

**AST.** Replace `Expr{Raw string}` with a real node tree
(`Expr` = `BinaryExpr | UnaryExpr | LitExpr | PathExpr | CallExpr | ListExpr |
MapExpr`). Keep `Raw` alongside for diagnostics / round-tripping / text diff.

**Evaluator.** New package `internal/wsl/eval` (or `engine/workflow/expr`):
`Eval(expr *Expr, scope Scope) (Value, error)` where `Value` is a typed union
(`Null, Bool, Int, Float, String, List, Map`). Rules:

- Arithmetic: `int op int → int`; any `float` operand → `float`; `%` ints only;
  divide-by-zero → error (routes to `on fail`).
- `+` on two strings → concatenation. `+` on string+non-string → error (use
  interpolation).
- Comparisons return `Bool`. `==` is deep-equal for scalars; cross-type numeric
  compare coerces int↔float; `string == int` → error.
- `&&` / `||` short-circuit and return the **truthiness-narrowed** result per
  decision #3: falsy = `null`, `false`, `0`, `0.0`, `""`, `[]`, `{}`; everything
  else truthy. `when x` is legal and means `when truthy(x)`. `!x` = `!truthy(x)`.
  Document the falsy set prominently — it's the one place truthiness bites.
- Unknown identifier → error at **eval** time, and a **diagnostic at validate
  time** for anything statically resolvable (constants, declared aliases/lets).

**`when` / `if` wiring.**

- `flow.CurrentTransition.OnSuccessWhen` / `.If` carry the compiled `*Expr`
  (serialise the tree, not the raw string, into the flow JSON — bump the flow
  schema version).
- Worker: `ok := eval(expr, scope); if err → on fail; if !ok → next 'on success
  when' candidate, else the unguarded 'on success', else 'on fail'`.
- **Multiple `on success when` semantics (define now):** candidates are evaluated
  **top to bottom, first true wins**. An unguarded `on success -> X` acts as the
  final `else` and must be last if present. `wsl_validate` errors on an
  unguarded success edge that isn't last, and warns on a set of guards with no
  unguarded fallback.

**String interpolation.** `"posting ${item.amount} to ${account.code}"` — the
literal parser splits on `${…}`, each hole is a full expression, result is
`String`. `<<token>>` stays supported (deprecation TBD, not this phase).

**Where expressions are allowed in Phase 1:** `when`, `if`, and inside string
literals. **Not yet** in action args or `const` (Phase 2) — keeps the blast
radius small.

ABI impact: **none** (evaluation is engine-side; flow JSON schema bumps, which is
an engine-internal format, not the transition wire protocol).

Test deliverables: section 7, tables 7.3 and 7.4 in full.

</details>

---

### Phase 2 — `let` bindings + expressions in action args + computed `const`

**`let name = <expr>` — DONE** (`engine-evolution`).

- Grammar: `let <ident> = <expr>` statements in the state body, after `if` and
  before `continue`/`skip`/`action`. `let` is matched as a contextual identifier
  (like `skip to`), no lexer keyword added. Several `let`s may chain and
  reference each other.
- The value text is sliced from source (not re-serialised from tokens) so
  arithmetic operators the WSL lexer does not tokenise survive into the
  expression string.
- CST `CSTLet` → AST `State.Lets []LetBinding` → IR `Node.Lets` → schema
  `tr["lets"]` → `domain.FlowTransition.Lets []FlowLet`.
- Build time: each value is parsed via `ParseExpr` (fails `wsl_validate` if
  malformed); re-assigning a name in the same state is a `SemanticError`
  (single-assignment).
- Runtime: `applyLetBindings()` runs in `ProcessState` right after the
  parallel-state check — before `if`, argument binding, the action call, and
  `when` guards — evaluating each binding in order against the live context and
  writing it back via `SetValue`, so later bindings, `if`, args and guards all
  see it.
- **SWSL** (`.swsl`): module-level `let name = <expr>` lines (alongside `const`),
  evaluated to the end of the source line. They attach to the workflow's start
  state, so they run once on entry. Same build-time validation + single-assignment
  check. `internal/wsl/simplified_wsl.go` `parseSimplifiedLet` +
  `buildSimplifiedAST`.
- Tests: `internal/wsl/expr_test.go` (`TestLetBindings_Parse` /
  `_RejectDuplicate` / `_RejectMalformed` / `_SWSL` / `_SWSL_RejectDuplicate`),
  `engine/workflow/worker_guards_test.go` `TestProcessState_LetBindings`
  (`base=10`, `total=base*rate=30`, guard `total >= 30` routes to `Big`).

**Expressions in action args + computed `const` — NOT STARTED.** Both are
grammar-level changes that ripple through the parser; they deserve their own
focused pass. Scoping notes:

- **Computed `const`** — `parseConstValue` (`internal/wsl/parser.go:198`) accepts
  exactly one scalar token / object / array. `timeout: base * 1000` doesn't
  parse today (`*` is unexpected after the scalar). Needs `parseConstValue` to
  fall into the expression parser when a scalar is followed by an operator, plus
  ordered evaluation of the const block with forward-reference detection. Risk:
  `const` parsing is used everywhere; bare identifiers currently coerce to
  strings (`convertScalarToken`), so `x: foo` changing meaning from `"foo"` to
  "the const foo" is a subtle breaking change — needs care.
- **Expression action args** — args are already captured as `Expr{Raw}`; they're
  `key: value` pairs where value can be a token or `<<ref>>`. Making value a full
  expression means `parseArg` → expression parser, `argsToOptions` /
  `mergeActionArgsIntoTransition` evaluating trees, and the worker resolving them
  to plain values before the transition call (wire protocol stays unchanged).
- **`let name = <expr>`** — new statement keyword in the state body grammar; CST
  `CSTLet`, AST `State.Lets []Binding`, IR carry, worker evaluates in order and
  writes into the session context before the action runs. Single-assignment
  check at build time.

The Phase 1 expression parser + evaluator + `Expr.Tree` plumbing are exactly the
foundation these need; the remaining work is grammar + wiring.

Original design:

- **`let name = <expr>`** as a statement inside a state, before the `action`
  line; and an optional workflow-level `let { a = …, b = a + 1 }` block
  evaluated once at workflow entry. Single-assignment; redefining `name` →
  validate error. Visible to every expression *after* its definition in flow
  order.
- **Action args become expressions:** `action m.Do(timeout: $constants.base * 2,
  label: "run ${input.id}")`. The engine evaluates each arg to a `Value` and
  puts the resolved value in the `config` map — **wire protocol unchanged**, the
  worker still gets a plain map.
- **Computed `const`:** `const { base: 30, timeout: base * 1000 }`. Evaluated in
  declaration order at parse/load time; forward references → validate error.
- Interaction with revisits: because `let` is single-assignment and args are
  re-evaluated each time a state runs, a revisited state recomputes arg
  expressions against current context (intended — that's how a `foreach`-free
  manual loop can still make progress via aliases). Document explicitly.

ABI impact: **none**.

---

### Phase 3 — `foreach` sequential + `foreach … parallel[limit]` — **DONE (first cut)** · `while` — not started

**`foreach` shipped** on `engine-evolution`. Syntax landed (slightly tighter than
the original sketch — one action per body, no per-iteration `on` edges yet):

```wsl
state PostLines {
  foreach line in <<invoice.lines>> {
    action ledger/ledger.Post(amount: line.amount) as posted
  }
  on success -> Done       # after all iterations
  on fail -> Rollback
}
```

- Grammar: `foreach <name> in <expr> { action ... }` in the state body (after
  `if`/`let`). `foreach` is a contextual identifier — no lexer keyword. The body
  is exactly one `action` (with optional `as alias`). A `foreach` state may not
  also carry a top-level `action` (build error).
- CST `CSTForEach` → AST `State.ForEach *ForEach{Var, List *Expr, Action}` (the
  body action is also `State.Action` so arg injection / alias / resolver
  handling is unchanged) → IR `Node.ForEach` → schema `foreach_var` /
  `foreach_list` → `domain.FlowTransition.ForEachVar` / `ForEachList`.
- Build time: the collection expression is parsed via `ParseExpr`
  (malformed → `wsl_validate` failure).
- Runtime (`engine/workflow/foreach.go` `processForEach`, dispatched from
  `ProcessState` right after `let`): evaluates the collection to a slice, then
  calls the body action once per element via `CallTransitionByName` (the
  `parallel.go` pattern) with `<var>` and `<var>_index` bound in the shared
  context each iteration. Bounded by slice length + the Phase 0 `MaxSteps` guard.
  - empty / null collection → 0 iterations → `on success`
  - non-list collection → error (400), no iterations
  - first iteration failure → stop, take `on fail` (`False`), aggregated error;
    no `on fail` → hard stop
  - full success → the action alias is bound to the ordered `[]response`, then
    the `on success` path (first truthy `on success when` guard, else the
    unguarded `on success`)
- Tests: `internal/wsl/expr_test.go` (`TestForEach_Parse` / `_RejectMalformedCollection`),
  `engine/workflow/foreach_test.go` (all-succeed routes to Done + alias is a
  3-slice + `line`/`line_index` bound; empty list; iteration-failure → Rollback
  stopping after the failure; non-list errors; full WSL→schema→`CorrectFlow`
  pipeline), `engine/workflow/worker_guards_test.go` build-reject test.
**`foreach … parallel[limit: K]` shipped** (bounded data fan-out — the Phase 4
"shape 3"). `foreach x in <xs> parallel[limit: 4] { action ... }` runs iterations
concurrently, at most K in flight (`parallel` with no `[limit]` = unbounded).

- Grammar: optional `parallel[limit: N]` between the collection expression and
  the `{`. `limit` must be `>= 1` (build error otherwise). CST
  `CSTForEach.Parallel*` → AST `ForEach.Parallel`/`ParallelLimit` → schema
  `foreach_parallel` / `foreach_limit` → `domain.FlowTransition.ForEach*`.
- Runtime (`runForEachParallel`): each iteration runs in its own snapshot
  context + branch worker/session (the `parallel.go` isolation pattern), with a
  semaphore of size K. Every iteration runs to completion even when some fail
  (no cancellation — matches existing `parallel` semantics). Ordered result
  slice; any failure → `on fail` with an aggregated error. Fresh transition
  instance per iteration via DI where available, else serialised on a per-loop
  mutex.
- Tests: `internal/wsl/expr_test.go` `TestForEach_ParallelParse` /
  `_ParallelRejectsBadLimit`; `foreach_test.go` — ordered results with real
  peak-concurrency == limit, unbounded shows overlap, failure → Rollback while
  all iterations still run. Race-tested.

- **Not done:** per-iteration `on success -> _ / on fail -> X` inside the body;
  `foreach x, i in …` index-name syntax; loop-var shadow lint; SWSL `foreach`.

**`while <expr> max: N` — not started.** Needs the mandatory `max:` guard and
per-iteration re-evaluation; its own increment.

<details>
<summary>Original Phase 3 design</summary>

**`foreach`** — the 90% case ("for each line item, post a ledger entry"):

```
state PostLines {
    foreach line in <<invoice.lines>> {
        action ledger.PostEntry(amount: line.amount, account: line.account) as posted
        on success -> _
        on fail -> Rollback
    }
    on success -> Done          # after all iterations
    on fail -> Rollback
}
```

- Binds `line` and `line_index` (name configurable: `foreach line, i in …`).
- Lowers to a controlled sub-execution: the engine iterates the list, running the
  block body per element, sharing the parent context plus the loop bindings.
- Empty list → zero iterations → `on success`.
- Iteration failure → break, take the block's `on fail` (default: propagate).
- Static bound: the list length is known at run time; combined with the Phase 0
  step budget this is safe. `wsl_validate` errors if `line` shadows an existing
  binding.

**`while`** — must carry an explicit guard:

```
state Drain {
    while <<queue.size>> > 0 max: 500 {
        action queue.Pop() as item
        on success -> _
        on fail -> Failed
    }
    on success -> Done
}
```

- `max:` is **mandatory** (validate error without it). Hitting `max` → config
  which: `on fail` (default) or `on success` via `on max -> State`.
- Guard re-evaluated before each iteration by the Phase 1 evaluator.

**`foreach … parallel`** — see Phase 4.

ABI impact: **none** (new syntax, engine-side execution).

</details>

---

### Phase 4 — generalise `parallel` — partly done

| Shape | Status |
|---|---|
| 1. `parallel[count: N]` replicated fan-out | ✅ pre-existing, kept |
| 2. heterogeneous `parallel { branch {…} branch {…} }` | **not started** |
| 3. data fan-out `foreach x in xs parallel[limit: K]` | ✅ **done** (delivered in Phase 3) |

**Rename decision (§2) — CLOSED: keep `parallel`.** The §2 analysis still holds
(audience is business-logic authors; renaming churns every published workflow for
a word). `foreach … parallel` reads naturally. No `concurrency` alias.

Remaining Phase 4 work is shape 2 (heterogeneous fork/join) — a new block
grammar with N independent sub-flows whose aliases merge back into the parent.
Its own increment.

<details>
<summary>Original Phase 4 design</summary>

Keep shape 1 (`parallel[count: N]`). Add:

**Heterogeneous fork/join:**

```
parallel Gather {
    branch { action pricing.Fetch() as price   on success -> _  on fail -> _ }
    branch { action tax.Fetch()     as tax     on success -> _  on fail -> _ }
    branch { action ship.Fetch()    as ship    on success -> _  on fail -> _ }
}
wait Gather { join Gather; on success -> Combine; on fail -> Failed }
```

- Each `branch` is an independent sub-flow with its own aliases merged back into
  the parent context on success (name-collision across branches → validate error).
- Failure aggregation identical to today's `parallel.go` behaviour.

**Data fan-out:**

```
foreach line in <<invoice.lines>> parallel[limit: 4] {
    action ledger.PostEntry(amount: line.amount) as posted
    on success -> _
    on fail -> _
}
wait; on success -> Done; on fail -> Rollback
```

- Bounded concurrency (`limit`), ordered result array bound to an alias, same
  join semantics.
- This is the construct that can eventually deprecate `parallel[count: N]`
  (`count: N` ≡ `foreach _ in range(N) parallel`).

**Rename decision (section 2) lands here** — one keyword pass, alias + deprecation
diagnostic if we change it.

ABI impact: **none**. Cancellation of in-flight branches on first failure is a
*possible* addition but changes observable behaviour (transitions may not
complete) — treat as a separate opt-in (`parallel[cancel_on_fail: true]`),
discuss separately.

</details>

---

### Phase 5 — `retry` — **DONE** · `timeout` — blocked (ABI)

**`retry` shipped** on `engine-evolution`:

```wsl
state ChargeCard {
  retry[max: 3, delay: "200ms", on: "err.retryable == true"]
  action payments/payments.Charge(amount: total) as charge
  on success -> Confirm
  on fail -> Failed
}
```

- Grammar: `retry[max: N, delay: "<dur>", on: "<expr>"]` state attribute (before
  the action, alongside `if`/`let`). `max` required (`>= 1`, = number of
  retries); `delay` optional Go duration string; `on` optional expression.
  `parseOptionalBracketAttrs` now accepts a few keywords (`on`, `error`, `if`,
  `when`) as attribute keys.
- Build time: `max` range-checked, `delay` validated with `time.ParseDuration`,
  `on` parsed via `ParseExpr`. `retry` on a `foreach` state → error (not yet
  supported).
- CST `CSTRetry` → AST `State.Retry *RetryPolicy` → IR → schema `retry` →
  `domain.FlowTransition.Retry *FlowRetry`.
- Runtime (`engine/workflow/retry.go` `callTransitionWithRetry`, wrapping the
  action call in `ProcessState`): after a failed attempt (call error, missing
  `FlowStepResult`, step error, or `Success == false`), if the `on` guard (with
  `err` bound to `{message, ...map response fields}`) is truthy or absent, sleep
  `delay` and re-run the action — up to `max` times. The rest of `ProcessState`
  then processes the final attempt's result unchanged, so success/fail routing,
  guards, aliases all work.
- Tests: `internal/wsl/expr_test.go` (`TestRetry_Parse` / `_ParseMinimal`),
  `engine/workflow/retry_test.go` (succeeds after transient failures with exactly
  N calls; exhausts → `on fail` with max+1 calls; `on` guard rejects → 1 call;
  `on` guard allows → retries; full WSL→schema→`CorrectFlow`; build-validation
  matrix; foreach+retry rejected).
- **Idempotency obligation:** a transition under a retry policy may be called
  more than once — must be idempotent. Belongs in the transition-authoring docs.

**`timeout` — blocked.** Enforcing a timeout means cancelling an in-flight
transition call. Transitions are `func(command, config, flags)` — **no context
parameter** — so the engine cannot cancel a running Go transition; a
goroutine-plus-`select` would abandon (leak) the goroutine on expiry. Doing this
properly requires adding a context/deadline to the transition signature, which
is an **ABI change of the highest severity** per `CLAUDE.md` §constraint #2.
That needs its own design discussion (cooperative deadline via the session
context is a possible non-breaking middle ground). Not attempted here.

ABI impact of `retry`: **none** (pure engine-side; wire protocol unchanged).

---

### Phase 6 — durable execution: `wait signal`, human-approval (SEPARATE TRACK)

This is the big one and **must not be bolted onto the above**. It requires:

- **Workflow-run persistence** — the ability to serialise a paused run
  (position, context, bindings, in-flight parallel groups) to storage and resume
  it later, possibly in a different process.
- A **signal/event ingress** API (registry API surface) to deliver
  `signal(runId, name, payload)`.
- `wait signal <name> [timeout: <dur>] -> State` as a state that suspends the run.
- Human-approval = `wait signal approval` + a transition that creates the
  approval task in the business system + a UI/endpoint that posts the signal.

Prerequisite design doc: **"Durable workflow execution & signals"** — separate
RFC. Do not start Phase 6 implementation until that lands. It *does* have ABI
implications (persisted-run format becomes a compatibility surface).

---

## 6. Companion work: transition-library audit (not an engine change)

Independent of the engine phases. Take the downstream "10 packages" and:

1. Classify each: which need *only* `foreach` + arithmetic (→ rewrite as WSL once
   Phase 1–3 land), which are irreducibly Go (Lua, binary protocols, HTTP,
   crypto, the workflow HTTP server).
2. For the irreducibly-Go ones, extract reusable primitives into `packages/`
   (`std-*`) as small single-purpose transitions per `CLAUDE.md` §"Core rule".
3. Publish to the registry so downstream apps compose instead of reimplement.

Deliverable: a table (package → verdict → target `std-*` home → tracking issue).

---

## 7. Testing strategy — "everything, every success and every failure"

### 7.1 Layers

| Layer | Location | What it covers |
|---|---|---|
| L1 Lexer | `internal/wsl/lexer_test.go` | every token; operator disambiguation (`==` vs `=`, `-` as minus vs ident, negative-number literals, `${` in strings) |
| L2 Parser / CST | `internal/wsl/parser_test.go`, `simplified_wsl_test.go` | every grammar production, success + every malformed form with expected diagnostic |
| L3 AST / IR lowering | `internal/wsl/build_ast_test.go`, `ir_test.go` | CST→AST→IR, every `SemanticError`, fork/join balance, `let` shadowing, loop-guard presence |
| L4 Expression evaluator | new `…/eval/eval_test.go` | operator truth tables, coercion, type errors, null, short-circuit, precedence, builtins, unknown-ident |
| L5 Executor / worker | `engine/workflow/*_test.go` | per construct: golden path + **every** `on fail` / error branch, using the fake-transition harness (`parallelTestTransition` pattern) |
| L6 End-to-end | `engine/workflow/engine_wsl_test.go` + `tests/` fixtures | WSL text → `Process` → assert final state, response, routed path |
| Fuzz | `internal/wsl/*_fuzz_test.go` (Go native) | lexer and expression parser must not panic on arbitrary input |
| Regression corpus | `tests/regressions/<issue>.wsl` | one fixture per fixed bug, asserted end-to-end |

### 7.2 The failure-matrix rule

For each construct, enumerate failure modes as a table in the test file and
require **one test per row**. Reviewer checks the table is exhaustive. Example
for `foreach`:

| # | Scenario | Expected |
|---|---|---|
| 1 | non-list value | validate error / runtime error → `on fail` |
| 2 | empty list | 0 iterations, `on success` |
| 3 | iteration N fails, no block `on fail` | propagate to state `on fail` |
| 4 | iteration N fails, block `on fail -> X` | route to X, remaining items skipped |
| 5 | loop var shadows existing binding | validate error |
| 6 | list length × body steps exceeds step budget | budget abort with trace |
| 7 | body has no terminating success edge | validate error |
| 8 | alias inside body read after loop | last-iteration value, documented |

### 7.3 Expression evaluator — required coverage (L4)

- **Per operator** (`+ - * / % == != < <= > >= && || ! unary-`): a truth/value
  table with int, float, mixed, string, bool, null operands — each cell either a
  value or a typed error.
- Precedence: `2 + 3 * 4 == 14`, `a || b && c` = `a || (b && c)`,
  `!a == b` = `(!a) == b`, unary minus vs subtraction.
- Short-circuit: RHS with a side-effecting builtin is not evaluated when LHS
  decides it (use a builtin that errors to prove it).
- Coercion: `1 == 1.0` true; `"1" == 1` error; `int("3")` = 3; `int("x")` error.
- Null: `null == null` true; `null + 1` error; `default(null, 5)` = 5;
  `has(obj.missing)` = false.
- Path resolution: missing constant, missing alias, missing nested field, index
  into list (in-range, out-of-range, negative), index into non-list.
- Interpolation: `"${a}${b}"`, nested `"${ outer(inner) }"`, unterminated `${`,
  literal `$` not followed by `{`.
- Unknown identifier → error; validate-time diagnostic for statically-known
  scopes.

### 7.4 `when` / `if` gating — required coverage (L5, currently missing entirely)

| # | Setup | Expected route |
|---|---|---|
| 1 | `on success when a == 1` , a=1 | true branch |
| 2 | same, a=2, unguarded `on success -> D` present | D |
| 3 | same, a=2, no unguarded success | `on fail` |
| 4 | two guards, first false second true | second's target |
| 5 | two guards, both true | first's target (top-down) |
| 6 | guard references missing alias | `on fail` + diagnostic |
| 7 | guard is `err.retryable == true` on the `on fail` edge | retry/fail routing |
| 8 | `if` false at state entry | `else` / skip per spec |
| 9 | unguarded `on success` not last | **validate error** |
| 10 | parent-flow reference in child guard | resolves against shared context |

### 7.5 Concurrency — required coverage (L5/L6)

Extend `engine/workflow/parallel_test.go`:

- `parallel[count:N]`: all succeed; branch K fails; branch K panics; N=0; N=1;
  wait without fork; two waits joining one fork (validate error); alias binds
  ordered array; `branch.index` visible per branch.
- Heterogeneous `parallel{branch…}`: all succeed & aliases merged; one branch
  fails → aggregated error; alias collision across branches (validate error);
  branch with its own internal `on fail`.
- `foreach…parallel[limit:K]`: concurrency never exceeds K (observable via a
  counting transition); ordered results; one element fails; empty list.

### 7.6 CI gate

- `make test` green in every module touched.
- New: coverage floor for `internal/wsl` and `engine/workflow/expr` (start at
  85%, ratchet up).
- Fuzz corpus runs in CI (short duration) + nightly (long).
- Every phase PR must add its failure-matrix table; a phase is not "done" until
  the table has no empty rows.

---

## 8. Sequencing summary

| Phase | Delivers | Text-format change | Wire-protocol (ABI) impact | Depends on |
|---|---|---|---|---|
| 0 | step budget, visit cap, `==` lexing, `when` deprecation diag | none | none | — |
| 1 | expression evaluator, real `when`/`if`, arithmetic, interpolation | additive | none | 0 |
| 2 | `let`, expression args, computed `const` | additive | none | 1 |
| 3 | `foreach`, `while max:` | additive | none | 1, (0) |
| 4 | heterogeneous `parallel`, `foreach…parallel`, rename decision | additive (+ alias if renamed) | none | 1, 3 |
| 5 | `retry`, `timeout` state attrs | additive | none (docs: idempotency) | 1 |
| 6 | durable execution, `wait signal`, human-approval | additive | **yes — persisted-run format** | separate RFC |

Phases 0–5 keep the WSL↔worker contract frozen. That is deliberate: it means all
of this can ship without a breaking-change event for the ecosystem, and the
"validated by engine vX.Y" badge is sufficient to communicate the new
capabilities.

---

## 9. Open decisions for the maintainer

1. `parallel` keyword: keep / rename to `concurrency` / other. (§2)
2. `<<token>>` vs `${…}`: deprecate `<<>>` eventually, or keep both forever? (§5.1)
3. Boolean strictness: require `Bool` operands for `&&`/`||` (recommended) or
   allow truthiness? (§5.1)
4. `foreach` binding syntax: `foreach x in list` + implicit `x_index`, or
   `foreach x, i in list`? (§5.3)
5. Step-budget defaults and whether they're per-profile or per-workflow. (§4)
6. Do we start the Phase 6 durable-execution RFC now in parallel, or after
   Phase 3? (§5.6)
