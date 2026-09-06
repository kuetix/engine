package workflow

import (
	"reflect"
	"time"

	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/logger"
)

// callTransitionWithRetry invokes the current state's action, honouring its
// `retry[max: N, delay: "..", on: ".."]` policy. It re-runs the action (not any
// downstream state) after a failed attempt, up to Max retries, optionally gated
// by the `on` expression evaluated with `err` bound to `{message: "..."}`.
//
// Retry is a pure engine-side concern; the transition wire protocol is
// unchanged. Transitions covered by a retry policy must be idempotent.
func (baseWorker *workflowWorker) callTransitionWithRetry(callPath string, wsc *WorkerSessionContext, mapping *ServiceTransitionMapping) ([]reflect.Value, error) {
	results, err := CallTransitionByName(callPath, wsc, mapping, boot.MetaFunctionCache)

	retry := wsc.Flow.CurrentTransition.Retry
	if retry == nil || retry.Max < 1 {
		return results, err
	}

	var delay time.Duration
	if retry.Delay != "" {
		delay, _ = time.ParseDuration(retry.Delay) // validated at build time
	}

	for attempt := 1; attempt <= retry.Max; attempt++ {
		if !attemptFailed(results, err) {
			return results, err
		}
		if retry.On != "" {
			wsc.SetValue("err", retryErrValue(results, err))
			pass := evaluateTransitionCondition(retry.On, wsc)
			if pass == nil || !*pass {
				logger.Debugf("[retry] %s: 'on' guard rejected retry after attempt %d", callPath, attempt)
				return results, err
			}
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		logger.Debugf("[retry] %s: retrying, attempt %d of %d", callPath, attempt+1, retry.Max+1)
		results, err = CallTransitionByName(callPath, wsc, mapping, boot.MetaFunctionCache)
	}
	return results, err
}

// attemptFailed reports whether a transition call is a failure worth retrying:
// a call error, no FlowStepResult, a step error, or an unsuccessful step.
func attemptFailed(results []reflect.Value, err error) bool {
	if err != nil {
		return true
	}
	sr, found := extractStepResult(results)
	if !found {
		return true
	}
	return sr.Error != nil || !sr.Success
}

func extractStepResult(results []reflect.Value) (domain.FlowStepResult, bool) {
	for _, r := range results {
		if sr, ok := r.Interface().(domain.FlowStepResult); ok {
			return sr, true
		}
	}
	return domain.FlowStepResult{}, false
}

// retryErrValue builds the `err` value the retry `on` expression inspects.
func retryErrValue(results []reflect.Value, err error) map[string]interface{} {
	msg := ""
	if err != nil {
		msg = err.Error()
	} else if sr, ok := extractStepResult(results); ok && sr.Error != nil {
		msg = sr.Error.Error()
	}
	out := map[string]interface{}{"message": msg}
	// If the step returned a map response, expose its fields too (so an
	// expression can read e.g. `err.retryable` or `err.code`).
	if sr, ok := extractStepResult(results); ok {
		if m, ok := sr.Response.(map[string]interface{}); ok {
			for k, v := range m {
				if k != "message" {
					out[k] = v
				}
			}
		}
	}
	return out
}
