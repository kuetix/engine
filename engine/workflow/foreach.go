package workflow

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
	"github.com/kuetix/logger"
)

// processForEach runs a `foreach <var> in <list> { action ... }` state: it
// evaluates the collection expression to a slice and calls the state's action
// once per element, with <var> and <var>_index bound in the session context.
//
// Iteration stops at the first failure and the state's `on fail` path is taken.
// On full success the action's alias (if any) is bound to the ordered slice of
// per-iteration responses and the `on success` path (guards first, then the
// unguarded success) is taken.
//
// The loop is bounded by the collection length; combined with the engine's
// MaxSteps guard this cannot run away.
func (baseWorker *workflowWorker) processForEach(session *WorkerSessionContext) (bool, string) {
	flow := session.Flow
	varName := flow.CurrentTransition.ForEachVar
	listExpr := flow.CurrentTransition.ForEachList
	callPath := strings.Split(flow.CurrentState.State, "#")[0]

	node, perr := parseConditionExpr(listExpr)
	if perr != nil {
		baseWorker.SetError(issues.NewIssue(fmt.Sprintf("foreach %s: %v", varName, perr), perr), http.StatusInternalServerError)
		return false, ""
	}
	raw, eerr := EvalExpr(node, sessionScope{session})
	if eerr != nil {
		baseWorker.SetError(issues.NewIssue(fmt.Sprintf("foreach %s in %q: %v", varName, listExpr, eerr), eerr), http.StatusInternalServerError)
		return false, ""
	}

	items, ok := toIterable(raw)
	if !ok {
		msg := fmt.Sprintf("foreach %s: collection %q is %s, not a list", varName, listExpr, typeName(raw))
		baseWorker.SetError(issues.NewIssue(msg, errors.New(msg)), http.StatusBadRequest)
		return false, ""
	}

	_, workerTransitions := baseWorker.resolveTransitionsForPath(callPath)
	if workerTransitions == nil {
		baseWorker.SetError(&issues.Issue{
			Message: fmt.Sprintf("foreach %s: transitions not found for %s, add its module to resolvers", varName, callPath),
		}, http.StatusInternalServerError)
		return false, ""
	}
	workerTransitions.SetWorkerSessionContext(session)

	responses := make([]interface{}, 0, len(items))
	for i, item := range items {
		session.SetValue(varName, item)
		session.SetValue(varName+"_index", int64(i))

		results, err := CallTransitionByName(callPath, session, workerTransitions, boot.MetaFunctionCache)
		if err != nil {
			return baseWorker.foreachFail(session, varName, i, err)
		}
		var stepResult domain.FlowStepResult
		found := false
		for _, r := range results {
			if sr, ok := r.Interface().(domain.FlowStepResult); ok {
				stepResult = sr
				found = true
				break
			}
		}
		if !found {
			return baseWorker.foreachFail(session, varName, i, fmt.Errorf("transition %s did not return domain.FlowStepResult", callPath))
		}
		if !stepResult.Success || stepResult.Error != nil {
			e := stepResult.Error
			if e == nil {
				e = fmt.Errorf("iteration %d failed", i)
			}
			return baseWorker.foreachFail(session, varName, i, e)
		}
		responses = append(responses, stepResult.Response)
	}

	if alias := flow.CurrentTransition.Response; alias != "" {
		session.SetValue(alias, responses)
	}
	baseWorker.SetResponse(responses)

	logger.Debugf("[foreach] %s: %d iterations succeeded", varName, len(items))
	target, okTarget := foreachSuccessTarget(flow, session)
	if !okTarget {
		return false, ""
	}
	return true, target
}

func (baseWorker *workflowWorker) foreachFail(session *WorkerSessionContext, varName string, index int, cause error) (bool, string) {
	flow := session.Flow
	baseWorker.SetError(&issues.Issue{
		Message: fmt.Sprintf("foreach %s: iteration %d failed: %v", varName, index, cause),
		Errors:  []error{cause},
		Json: map[string]interface{}{
			"foreach":         varName,
			"iteration":       index,
			"flow.ConfigName": flow.ConfigName,
		},
	}, http.StatusInternalServerError)
	if flow.CurrentTransition.False != "" {
		return true, flow.CurrentTransition.False
	}
	return false, ""
}

// foreachSuccessTarget resolves where a fully-successful foreach state routes:
// the first truthy `on success when` guard, else the unguarded `on success`.
func foreachSuccessTarget(flow *domain.Flow, session *WorkerSessionContext) (string, bool) {
	for _, g := range flow.CurrentTransition.Guards {
		pass := evaluateTransitionCondition(g.When, session)
		if pass == nil {
			return "", false
		}
		if *pass {
			return g.To, true
		}
	}
	if flow.CurrentTransition.OnSuccessWhen != nil {
		pass := evaluateTransitionCondition(*flow.CurrentTransition.OnSuccessWhen, session)
		if pass == nil {
			return "", false
		}
		if !*pass {
			if flow.CurrentTransition.False != "" {
				return flow.CurrentTransition.False, true
			}
			return "", false
		}
	}
	return flow.CurrentTransition.True, true
}

// toIterable normalises a value into a slice for iteration.
func toIterable(v interface{}) ([]interface{}, bool) {
	switch x := normalizeValue(v).(type) {
	case []interface{}:
		return x, true
	case nil:
		return nil, true // empty iteration
	}
	return nil, false
}
