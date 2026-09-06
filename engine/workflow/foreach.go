package workflow

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
	"github.com/kuetix/logger"
)

// processForEach runs a `foreach <var> in <list> [parallel[limit: K]] { action ... }`
// state: it evaluates the collection expression to a slice and calls the state's
// action once per element, with <var> and <var>_index bound in scope.
//
// Sequential mode stops at the first failure and takes the state's `on fail`
// path. Parallel mode runs all iterations (bounded by K, 0 = unbounded), waits
// for every one, then reports success only if all succeeded.
//
// On full success the action's alias (if any) is bound to the ordered slice of
// per-iteration responses and the `on success` path (guards first, then the
// unguarded success) is taken. The loop is bounded by the collection length;
// with the engine's MaxSteps guard it cannot run away.
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

	resolverKey, workerTransitions := baseWorker.resolveTransitionsForPath(callPath)
	if workerTransitions == nil {
		baseWorker.SetError(&issues.Issue{
			Message: fmt.Sprintf("foreach %s: transitions not found for %s, add its module to resolvers", varName, callPath),
		}, http.StatusInternalServerError)
		return false, ""
	}

	var responses []interface{}
	var failIndex int
	var failErr error
	if flow.CurrentTransition.ForEachParallel {
		responses, failIndex, failErr = baseWorker.runForEachParallel(session, varName, callPath, resolverKey, workerTransitions, items, flow.CurrentTransition.ForEachLimit)
	} else {
		responses, failIndex, failErr = baseWorker.runForEachSequential(session, varName, callPath, workerTransitions, items)
	}

	if failErr != nil {
		return baseWorker.foreachFail(session, varName, failIndex, failErr)
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

// runForEachSequential calls the body action once per element in order, stopping
// at the first failure.
func (baseWorker *workflowWorker) runForEachSequential(session *WorkerSessionContext, varName, callPath string, mapping *ServiceTransitionMapping, items []interface{}) ([]interface{}, int, error) {
	mapping.SetWorkerSessionContext(session)
	responses := make([]interface{}, 0, len(items))
	for i, item := range items {
		session.SetValue(varName, item)
		session.SetValue(varName+"_index", int64(i))
		resp, err := callForEachBody(callPath, session, mapping)
		if err != nil {
			return responses, i, err
		}
		responses = append(responses, resp)
	}
	return responses, 0, nil
}

// runForEachParallel runs every iteration concurrently (bounded by limit, 0 =
// unbounded), each in its own snapshot context, then waits for all of them.
func (baseWorker *workflowWorker) runForEachParallel(session *WorkerSessionContext, varName, callPath, resolverKey string, shared *ServiceTransitionMapping, items []interface{}, limit int) ([]interface{}, int, error) {
	n := len(items)
	responses := make([]interface{}, n)
	errsByIndex := make([]error, n)

	var sem chan struct{}
	if limit > 0 {
		sem = make(chan struct{}, limit)
	}
	var wg sync.WaitGroup
	var sharedMu sync.Mutex

	for i := 0; i < n; i++ {
		wg.Add(1)
		if sem != nil {
			sem <- struct{}{}
		}
		go func(index int, item interface{}) {
			defer wg.Done()
			if sem != nil {
				defer func() { <-sem }()
			}
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[foreach] %s iteration %d: recovered: %v", varName, index, r)
					logger.Debugf("Stack trace: %s", string(debug.Stack()))
					errsByIndex[index] = fmt.Errorf("panic in iteration %d: %v", index, r)
				}
			}()

			branchCtx := make(map[string]interface{}, len(*baseWorker.WorkflowContext.Context())+2)
			for k, v := range *baseWorker.WorkflowContext.Context() {
				branchCtx[k] = v
			}
			branchCtx[varName] = item
			branchCtx[varName+"_index"] = int64(index)
			branchContext := NewWorkflowContext(&branchCtx)

			branchFlow := *session.Flow
			branchWorker := &workflowWorker{
				WorkflowContext:       branchContext,
				Transitions:           baseWorker.Transitions,
				TransitionsResolved:   baseWorker.TransitionsResolved,
				TransitionsNamespaces: baseWorker.TransitionsNamespaces,
				Response:              &WorkerResponse{},
				LastResponse:          &WorkerResponse{},
				Debug:                 baseWorker.Debug,
				Options:               baseWorker.Options,
			}
			branchSession := &WorkerSessionContext{
				Engine:          session.Engine,
				Flow:            &branchFlow,
				Worker:          branchWorker,
				WorkflowContext: branchContext,
				ServerContext:   session.ServerContext,
				Parser:          NewParser(),
			}
			branchCtx["workflow.Flow"] = &branchFlow
			branchCtx["workflow.Worker"] = branchWorker

			mapping := shared
			if resolverKey != "" && di.CanResolve(defines.TransitionPrefix+resolverKey) {
				fresh := di.Resolve(defines.TransitionPrefix + resolverKey).(ServiceTransitionMapping)
				mapping = &fresh
			} else {
				sharedMu.Lock()
				defer sharedMu.Unlock()
			}
			mapping.SetWorkerSessionContext(branchSession)

			resp, err := callForEachBody(callPath, branchSession, mapping)
			if err != nil {
				errsByIndex[index] = err
				return
			}
			responses[index] = resp
		}(i, items[i])
	}
	wg.Wait()

	for i, err := range errsByIndex {
		if err != nil {
			return responses, i, err
		}
	}
	return responses, 0, nil
}

// callForEachBody invokes the loop body action once and extracts its result.
func callForEachBody(callPath string, session *WorkerSessionContext, mapping *ServiceTransitionMapping) (interface{}, error) {
	results, err := CallTransitionByName(callPath, session, mapping, boot.MetaFunctionCache)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		if sr, ok := r.Interface().(domain.FlowStepResult); ok {
			if !sr.Success || sr.Error != nil {
				if sr.Error != nil {
					return nil, sr.Error
				}
				return nil, errors.New("iteration failed")
			}
			return sr.Response, nil
		}
	}
	return nil, fmt.Errorf("transition %s did not return domain.FlowStepResult", callPath)
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
