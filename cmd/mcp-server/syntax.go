package main

// syntaxReference is the WSL/SWSL cheat sheet returned by the
// wsl_syntax_reference tool. Keep in sync with the wsl-skills plugin
// (write-wsl / write-swsl) and docs.kuetix.com/docs/wsl/specification.mdx.
const syntaxReference = `# WSL & SWSL Syntax Reference

WSL (Workflow Specific Language) is the workflow language used by the
kuetix/engine. It comes in two surface forms:

  - .wsl  — verbose, explicit state machines
  - .swsl — simplified, chained form

## Hierarchy

solution > feature > workflow

  - workflow : atomic unit, executes service actions
  - feature  : orchestrates workflows
  - solution : orchestrates features and workflows

## .wsl skeleton

    module <module_name>

    import <service/path>

    const {
        key: "value",
        nested: { inner: "value" }
    }

    workflow <Name> {
      start: <StartState>

      state <StateName> {
        action <service/path.Method>(param: value) as <Alias>
        on success -> <NextState>
      }

      state <FinalState> {
        action <service/path.Method>()
        end ok
      }
    }

## State attributes (in this order, before the action)

  - if <expr>                           : skip the state (take 'else') when falsy
  - let <name> = <expr>                  : bind a value; several may chain, each
                                          may reference earlier ones + context.
                                          Single-assignment.
  - retry[max: N, delay: "200ms", on: "<expr>"]
                                          : re-run the action after a failure,
                                          up to N times. 'delay' optional Go
                                          duration; 'on' optional expr (err is
                                          bound to {message, ...}) — falsy stops.
  - foreach <name> in <expr> [parallel[limit: K]] { action ... }
                                          : run the body action once per list
                                          element (<name> and <name>_index bound).
                                          parallel[limit: K] runs them K-at-a-time
                                          (bare 'parallel' = unbounded). Alias is
                                          bound to the ordered result list.
  - while[max: N] <expr> { action ... }  : re-run the body while <expr> is truthy,
                                          at most N iterations (max is required).
                                          while_index is bound each iteration.
  - continue on fail                     : proceed even if action errors
  - skip to                              : skip state under certain conditions
  - State parameters                     : state Foo(PriorAlias) { ... }

A 'foreach' / 'while' state has no top-level 'action' (the body holds it).

## Transitions

  - on success -> Next
  - on success -> Next(Alias)
  - on success when <expr> -> Next        (guarded; see below)
  - on error -> ErrorState                (alias: on fail)
  - end ok | end fail | end error

## When expressions (these actually gate at runtime)

Several 'on success when' guards are evaluated top-to-bottom; the first whose
expression is truthy wins. A plain 'on success' is the fallback.

    on success when result.status == "completed" -> Confirmed
    on success when result.status == "pending"   -> WaitMore
    on success                                   -> Unknown
    on fail -> Failed

## Expressions

Used in when / if / let / retry.on / while / foreach collection.

  - operators:  == != < <= > >=   + - * / %   && || !   ??
  - literals:   1  3.14  "str"  'str'  true  false  null  [a, b]  {k: v}
  - paths:      result.ok   constants.version   err.message   line.amount
  - <<path>>    also valid (equivalent to a bare path)
  - strings:    "posting ${item.amount} to ${account.code}"   ($ interpolation)
  - builtins:   len upper lower contains startsWith endsWith
                int float string bool default has
  - truthiness: falsy = null, false, 0, "", [], {} ; everything else truthy.
                'when x' means 'when truthy(x)'.
  - int / int  -> int  (promotes to float on a non-even division); '+' on two
    strings concatenates; string + number is an error (use "${...}").

## Variable references

  - $constants.key            (deep nesting: $constants.cfg.timeout)
  - $Alias.field              (arrays: $Alias.items[0].name)
  - $ParamName                (state parameters)
  - $error.message            (error context)

## Action arguments

  - Named:    action svc.M(key: "value", num: 42)
  - Object:   action svc.M(config: { timeout: 5000 })
  - Array:    action svc.M(items: ["a", "b"])
  - Var ref:  action svc.M(value: $constants.key)
  - Coerce:   action svc.M(code: $value|int)

## Loops and retry — examples

    state PostLines {
      foreach line in <<invoice.lines>> parallel[limit: 4] {
        action ledger/ledger.Post(amount: line.amount) as posted
      }
      on success -> Done
      on fail -> Rollback
    }

    state Drain {
      while[max: 500] <<queue.size>> > 0 {
        action queue/queue.Pop() as item
      }
      on success -> Done
      on fail -> Failed
    }

    state ChargeCard {
      retry[max: 3, delay: "200ms", on: "err.retryable == true"]
      action payments/payments.Charge(amount: total) as charge
      on success -> Confirm
      on fail -> Failed
    }

    state PriceLine {
      let base  = 10
      let total = base * $constants.rate
      action ledger/ledger.Post(amount: total) as posted
      on success when total >= 100 -> BigPosting
      on success -> Done
      on fail -> Failed
    }

## Execution guards

The engine aborts a run that exceeds ~10000 state transitions, or enters any
one state more than ~1000 times (a runaway loop). foreach/while are bounded by
their collection length / 'max'.

## Orchestration

    feature my_feature {
      start: Step1
      state Step1 { action workflow basic_step;   on success -> Step2 }
      state Step2 { action workflow process_data; end ok }
    }

    solution my_solution {
      start: Init
      state Init { action feature my_feature; on success -> Done }
      state Done { end ok }
    }

## Constants

Auto-typed values: strings, integers, floats, booleans, null. Nested
objects and arrays supported.

    const {
      headers: [{ key: "Content-Type", value: "application/json" }],
      config: {
        limits: { maxAmount: 10000, currencies: ["USD", "EUR"] }
      }
    }

## SWSL (simplified) form

SWSL chains states with the -> operator and binds errors with <-.
Terminal states end with a period.

    module my_module
    import service/path

    workflow hello :
      service/path.Greet(name: "world") as G
      -> service/path.Echo(msg: $G.text)
      <- service/path.LogError(err: $error.message)
      .

## Comments

    // single line
    # also single line

## Authoring guidelines

  - Place workflow files under runtime/workflows/<name>/.
  - State names in PascalCase.
  - Always define a start: state.
  - Terminal states must use end ok or end fail.
  - Non-terminal states need at least one on success -> transition.
  - Use 'as Alias' to reference action results downstream.
  - Import only the service paths actually used.
  - Module name typically matches the directory/file name.
`
