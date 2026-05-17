package main

// syntaxReference is the WSL/SWSL cheat sheet returned by the
// wsl_syntax_reference tool. Keep in sync with .claude/skills/write-wsl.md
// and .claude/skills/write-swsl.md.
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

## State attributes

  - if <expression>     : conditional execution guard
  - continue on fail    : proceed even if action errors
  - skip to             : skip state under certain conditions
  - State parameters    : state Foo(PriorAlias) { ... }

## Transitions

  - on success -> Next
  - on success -> Next(Alias)
  - on success when <expr> -> Next
  - on error -> ErrorState
  - end ok | end fail | end error

## When expressions

    on success when $constants.version == "1.0.0" -> VersionOneHandler
    on success when $constants.maxRetries > 2     -> HighRetryPath
    on success                                    -> DefaultHandler

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
