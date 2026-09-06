package wsl

import "fmt"

// AST semantic model

type Module struct {
	Name      string
	Imports   []Import
	Extends   []string
	Context   []Field
	Constants []Constant
	Workflows []Workflow
}

type Import struct {
	Path string
	As   string // optional alias
}

type Field struct {
	Name string
	Type string
}

type Constant struct {
	Name  string
	Value interface{} // can be string, number, bool, map[string]interface{}, []interface{}
}

type Workflow struct {
	Name   string
	Type   string // workflow type (workflow, feature, solution, etc.)
	Start  string
	States map[string]*State
}

// LetBinding is a `let name = <expr>` binding evaluated (in order) when the
// state is entered, before its `if`, arguments, and action.
type LetBinding struct {
	Name string
	Expr *Expr
}

// ForEach is a `foreach <Var> in <List> { <Action> }` loop. The engine
// evaluates List to a slice, then runs Action once per element with <Var> and
// <Var>_index bound in scope. Iteration stops on the first failure (the state's
// `on fail` path is taken); on full success the state's `on success` path is
// taken and the action alias (if any) is bound to the ordered result slice.
// RetryPolicy re-runs a state's action after a failure. Max is the number of
// retries (extra attempts). Delay is a Go duration string ("" = no delay). On,
// when set, is an expression evaluated after a failed attempt with `err` bound
// (`{message: "..."}`); a falsy result stops retrying.
type RetryPolicy struct {
	Max   int
	Delay string
	On    *Expr
}

// WhileLoop re-runs a state's action while Cond is truthy, at most Max times.
// Reaching Max with Cond still truthy takes the state's `on fail` path.
type WhileLoop struct {
	Max    int
	Cond   *Expr
	Action *Action
}

type ForEach struct {
	Var    string
	List   *Expr
	Action *Action
	// Parallel runs iterations concurrently. ParallelLimit caps in-flight
	// iterations (0 = unbounded). Sequential when Parallel is false.
	Parallel      bool
	ParallelLimit int
}

type State struct {
	Name           string
	Params         []string
	Action         *Action
	Transitions    []Transition
	Lets           []LetBinding
	ForEach        *ForEach
	While          *WhileLoop
	Retry          *RetryPolicy
	Start          bool
	End            *End
	IfExpr         *Expr // optional if condition expression
	ContinueOnFail bool  // continue on fail flag
	SkipTo         bool  // skip to flag
	// Parallel fork: run the state's action ParallelCount times concurrently.
	Parallel      bool
	ParallelCount int
	// Wait (join) state: block until all branches of JoinTarget finish.
	Wait       bool
	JoinTarget string
}

type Action struct {
	Module string // left part of qname if any
	Name   string // right part of qname
	Args   []Expr
	As     string // alias for result
}

type Transition struct {
	Name      string
	Condition Condition
	Start     bool
	WhenExpr  *Expr // optional when condition expression
	Target    string
	Args      []Expr
}

type End struct {
	Kind string            // "ok" or "fail"
	Attr map[string]string // attributes from end statement
}

// Condition kind
const (
	CondSuccess = "success"
	CondError   = "error"
	CondElse    = "else"
	CondExpr    = "expr"
)

type Condition struct {
	Kind string // one of constants above
	Expr *Expr  // present if Kind==CondExpr
}

// Expr carries the raw text of a WSL expression and, when it has been parsed,
// its typed tree (see expr.go). Tree is nil for expressions that are not parsed
// at build time (e.g. action arguments, which may be `key: value` pairs).
type Expr struct {
	Raw  string
	Tree ExprNode
}

// parseValidatedExpr parses raw expression text and returns a *Expr carrying
// both the text and the tree. A parse failure is a SemanticError naming the
// context (e.g. "when condition in state 'X'").
func parseValidatedExpr(raw, context string) (*Expr, error) {
	e := &Expr{Raw: raw}
	tree, err := ParseExpr(raw)
	if err != nil {
		return nil, &SemanticError{Msg: fmt.Sprintf("%s: %v", context, err)}
	}
	e.Tree = tree
	return e, nil
}

// IR Graph for visualization/runtime

type Graph struct {
	WorkflowName string
	WorkflowType string // workflow type (workflow, feature, application, etc.)
	Nodes        map[string]*Node
	Start        string
	Constants    map[string]interface{}
}

type Node struct {
	Name           string
	Action         *Action
	ParamNames     []string
	Edges          []Edge
	Start          bool
	Terminal       bool
	TerminalKind   string            // ok|fail
	Attr           map[string]string // for end nodes
	IfExpr         *Expr             // optional if condition expression
	Lets           []LetBinding      // let bindings evaluated on state entry
	ForEach        *ForEach          // optional foreach loop body
	While          *WhileLoop        // optional while loop body
	Retry          *RetryPolicy      // optional retry policy for the action
	ContinueOnFail bool              // continue on fail flag
	SkipTo         bool              // skip to flag
	// Parallel fork/join
	Parallel      bool
	ParallelCount int
	Wait          bool
	JoinTarget    string
}

type Edge struct {
	Condition Condition
	WhenExpr  *Expr // optional when condition expression
	To        string
	Args      []Expr
}
