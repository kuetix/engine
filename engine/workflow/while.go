package workflow

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
	"github.com/kuetix/logger"
)

// processWhile runs a `while[max: N] <expr> { action ... }` state: it
// re-evaluates the condition before each iteration and calls the state's action
// while it is truthy, up to N iterations. `while_index` (0-based) is bound in
// scope each iteration.
//
//   - condition falsy         -> `on success`
//   - action fails            -> `on fail`
//   - N iterations exhausted   -> `on fail` (guard still truthy)
//
// The mandatory `max` plus the engine's MaxSteps guard bound the loop.
func (baseWorker *workflowWorker) processWhile(session *WorkerSessionContext) (bool, string) {
	flow := session.Flow
	condRaw := flow.CurrentTransition.WhileCond
	max := flow.CurrentTransition.WhileMax
	callPath := strings.Split(flow.CurrentState.State, "#")[0]

	node, perr := parseConditionExpr(condRaw)
	if perr != nil {
		baseWorker.SetError(issues.NewIssue(fmt.Sprintf("while: %v", perr), perr), http.StatusInternalServerError)
		return false, ""
	}

	_, mapping := baseWorker.resolveTransitionsForPath(callPath)
	if mapping == nil {
		baseWorker.SetError(&issues.Issue{
			Message: fmt.Sprintf("while: transitions not found for %s, add its module to resolvers", callPath),
		}, http.StatusInternalServerError)
		return false, ""
	}
	mapping.SetWorkerSessionContext(session)

	truthy := func() (bool, bool) {
		v, err := EvalExpr(node, sessionScope{session})
		if err != nil {
			baseWorker.SetError(issues.NewIssue(fmt.Sprintf("while: condition %q: %v", condRaw, err), err), http.StatusInternalServerError)
			return false, false
		}
		return Truthy(v), true
	}

	iterations := 0
	for iter := 0; iter < max; iter++ {
		session.SetValue("while_index", int64(iter))
		cond, ok := truthy()
		if !ok {
			return false, ""
		}
		if !cond {
			logger.Debugf("[while] condition false after %d iterations", iterations)
			return whileDone(flow, session)
		}
		if _, err := callForEachBody(callPath, session, mapping); err != nil {
			return baseWorker.whileFail(session, iter, err)
		}
		iterations++
	}

	// exhausted max — did we stop because the guard went false, or because we
	// ran out of budget?
	session.SetValue("while_index", int64(max))
	cond, ok := truthy()
	if !ok {
		return false, ""
	}
	if cond {
		return baseWorker.whileFail(session, max, errors.New("reached max iterations with the condition still true"))
	}
	return whileDone(flow, session)
}

func (baseWorker *workflowWorker) whileFail(session *WorkerSessionContext, iter int, cause error) (bool, string) {
	flow := session.Flow
	baseWorker.SetError(&issues.Issue{
		Message: fmt.Sprintf("while: iteration %d failed: %v", iter, cause),
		Errors:  []error{cause},
		Json: map[string]interface{}{
			"while":           flow.CurrentTransition.WhileCond,
			"iteration":       iter,
			"flow.ConfigName": flow.ConfigName,
		},
	}, http.StatusInternalServerError)
	if flow.CurrentTransition.False != "" {
		return true, flow.CurrentTransition.False
	}
	return false, ""
}

func whileDone(flow *domain.Flow, session *WorkerSessionContext) (bool, string) {
	target, ok := foreachSuccessTarget(flow, session)
	if !ok {
		return false, ""
	}
	return true, target
}
