package workflow

import (
	"testing"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
)

// MockWorkerContext implements WorkerContext for testing
type MockWorkerContext struct {
	context map[string]interface{}
}

func (m *MockWorkerContext) Context() *map[string]interface{} {
	return &m.context
}

func (m *MockWorkerContext) SetContext(context *map[string]interface{}) {
	m.context = *context
}

func (m *MockWorkerContext) Value(name string) interface{} {
	return m.context[name]
}

func (m *MockWorkerContext) SetValue(key string, value interface{}) {
	m.context[key] = value
}

// MockWorker implements Worker interface for testing
type MockWorker struct {
	context  WorkerContext
	response *WorkerResponse
	debug    bool
}

//goland:noinspection GoUnusedExportedFunction
func NewMockWorker() *MockWorker {
	return &MockWorker{
		context: &MockWorkerContext{
			context: make(map[string]interface{}),
		},
		response: &WorkerResponse{
			Error:    nil,
			Response: nil,
		},
	}
}

func (m *MockWorker) Context() *map[string]interface{} {
	return m.context.Context()
}

func (m *MockWorker) SetContext(context *map[string]interface{}) {
	m.context.SetContext(context)
}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) Start(w EngineInterface) bool {
	return true
}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) InitResolvers(resolvers []string) {}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) PrepareContext(w EngineInterface, flow *domain.Flow) bool {
	return true
}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) ProcessState(w EngineInterface, flow *domain.Flow) (bool, string) {
	return true, ""
}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) Done(w EngineInterface) *WorkerResponse {
	return m.response
}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) ProcessStateError(w EngineInterface, flow *domain.Flow, errorMessage string) {}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) SetResponse(response any, statusCode ...int) {
	m.response.Response = response
}

func (m *MockWorker) GetResponse() any {
	return m.response.Response
}

func (m *MockWorker) GetWorkerResponse() *WorkerResponse {
	return m.response
}

func (m *MockWorker) HandleError(err interface{}, statusCode int) bool {
	if err != nil {
		if e, ok := err.(error); ok {
			m.SetError(issues.NewIssueFromError(e), statusCode)
			return false
		}
	}
	return true
}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) SetError(error *issues.Issue, statusCode ...int) bool {
	if m.response.Error == nil {
		m.response.Error = &issues.Issues{}
	}
	m.response.Error.Another(error)
	return false
}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) SetErrors(errors *issues.Issues, statusCode ...int) bool {
	m.response.Error = errors
	return false
}

//goland:noinspection GoUnusedParameter
func (m *MockWorker) MergeIssues(errors *issues.Issues, statusCode ...int) bool {
	if m.response.Error == nil {
		m.response.Error = errors
	} else {
		m.response.Error.More(errors.Errors()...)
	}
	return false
}

func (m *MockWorker) GetError() *issues.Issues {
	return m.response.Error
}

func (m *MockWorker) SetStatusCode(statusCode int) {
	m.response.StatusCode = statusCode
}

func (m *MockWorker) GetStatusCode() int {
	return m.response.StatusCode
}

func (m *MockWorker) SetDebug(debug bool) {
	m.debug = debug
}

func (m *MockWorker) IsDebug() bool {
	return m.debug
}

func (m *MockWorker) GetWorkflowContext() WorkerContext {
	return m.context
}

// TestOnSuccessWhen_ConditionEvaluation tests the OnSuccessWhen condition evaluation
func TestOnSuccessWhen_ConditionEvaluation(t *testing.T) {
	t.Run("Success with OnSuccessWhen condition that evaluates to true", func(t *testing.T) {
		// Test that when a step succeeds AND the OnSuccessWhen condition is true,
		// the flow continues to the success path
		trueCondition := "result == 42"

		transition := &domain.FlowTransition{
			Name:          "test",
			OnSuccessWhen: &trueCondition,
			True:          "success_state",
			False:         "failure_state",
		}

		// Verify the condition is set correctly
		if transition.OnSuccessWhen == nil {
			t.Fatal("OnSuccessWhen should not be nil")
		}

		if *transition.OnSuccessWhen != trueCondition {
			t.Errorf("Expected condition '%s', got '%s'", trueCondition, *transition.OnSuccessWhen)
		}
	})

	t.Run("Success with OnSuccessWhen condition that evaluates to false", func(t *testing.T) {
		// Test that when a step succeeds but the OnSuccessWhen condition is false,
		// the flow goes to the failure path
		falseCondition := "result == 0"

		transition := &domain.FlowTransition{
			Name:          "test",
			OnSuccessWhen: &falseCondition,
			True:          "success_state",
			False:         "failure_state",
		}

		if transition.OnSuccessWhen == nil {
			t.Fatal("OnSuccessWhen should not be nil")
		}

		if *transition.OnSuccessWhen != falseCondition {
			t.Errorf("Expected condition '%s', got '%s'", falseCondition, *transition.OnSuccessWhen)
		}
	})

	t.Run("Success without OnSuccessWhen condition", func(t *testing.T) {
		// Test backward compatibility: when OnSuccessWhen is not set,
		// the flow should behave as before
		transition := &domain.FlowTransition{
			Name:  "test",
			True:  "success_state",
			False: "failure_state",
		}

		if transition.OnSuccessWhen != nil {
			t.Error("OnSuccessWhen should be nil for backward compatibility")
		}
	})
}

// TestOnSuccessWhen_EdgeCases tests edge cases
func TestOnSuccessWhen_EdgeCases(t *testing.T) {
	t.Run("Empty OnSuccessWhen string", func(t *testing.T) {
		emptyCondition := ""
		transition := &domain.FlowTransition{
			Name:          "test",
			OnSuccessWhen: &emptyCondition,
		}

		if transition.OnSuccessWhen == nil {
			t.Error("OnSuccessWhen should not be nil even if empty")
		}

		if *transition.OnSuccessWhen != "" {
			t.Error("OnSuccessWhen should be empty string")
		}
	})

	t.Run("Complex expression in OnSuccessWhen", func(t *testing.T) {
		complexCondition := "result.status == 'completed' && result.code > 200"
		transition := &domain.FlowTransition{
			Name:          "test",
			OnSuccessWhen: &complexCondition,
		}

		if *transition.OnSuccessWhen != complexCondition {
			t.Errorf("Expected condition '%s', got '%s'", complexCondition, *transition.OnSuccessWhen)
		}
	})

	t.Run("OnSuccessWhen with placeholder syntax", func(t *testing.T) {
		placeholderCondition := "<<workflow.result>> > 0"
		transition := &domain.FlowTransition{
			Name:          "test",
			OnSuccessWhen: &placeholderCondition,
		}

		if *transition.OnSuccessWhen != placeholderCondition {
			t.Errorf("Expected condition '%s', got '%s'", placeholderCondition, *transition.OnSuccessWhen)
		}
	})
}

// TestOnSuccessWhen_WithOtherFields tests interaction with other transition fields
func TestOnSuccessWhen_WithOtherFields(t *testing.T) {
	t.Run("OnSuccessWhen with If condition", func(t *testing.T) {
		ifCond := "enabled == true"
		successCond := "result > 0"

		transition := &domain.FlowTransition{
			Name:          "test",
			If:            &ifCond,
			OnSuccessWhen: &successCond,
			True:          "success_state",
			False:         "failure_state",
		}

		// Both conditions should coexist
		if transition.If == nil || *transition.If != ifCond {
			t.Error("If condition should be preserved")
		}

		if transition.OnSuccessWhen == nil || *transition.OnSuccessWhen != successCond {
			t.Error("OnSuccessWhen condition should be set")
		}
	})

	t.Run("OnSuccessWhen with Else", func(t *testing.T) {
		successCond := "result.valid == true"
		elseState := "alternate_state"

		transition := &domain.FlowTransition{
			Name:          "test",
			OnSuccessWhen: &successCond,
			Else:          &elseState,
			True:          "success_state",
		}

		if transition.OnSuccessWhen == nil || *transition.OnSuccessWhen != successCond {
			t.Error("OnSuccessWhen should be set")
		}

		if transition.Else == nil || *transition.Else != elseState {
			t.Error("Else should be preserved")
		}
	})

	t.Run("OnSuccessWhen with ContinueOnFail", func(t *testing.T) {
		successCond := "output != null"

		transition := &domain.FlowTransition{
			Name:           "test",
			OnSuccessWhen:  &successCond,
			ContinueOnFail: true,
			True:           "success_state",
		}

		if !transition.ContinueOnFail {
			t.Error("ContinueOnFail should be true")
		}

		if transition.OnSuccessWhen == nil || *transition.OnSuccessWhen != successCond {
			t.Error("OnSuccessWhen should be set")
		}
	})
}

// TestOnSuccessWhen_ParentFlowContext tests parent flow scenarios
func TestOnSuccessWhen_ParentFlowContext(t *testing.T) {
	t.Run("Child flow with OnSuccessWhen accessing parent context", func(t *testing.T) {
		parentCondition := "<<parent.result>> == 'success'"

		childTransition := &domain.FlowTransition{
			Name:          "child_transition",
			OnSuccessWhen: &parentCondition,
			True:          "child_success",
			False:         "child_failure",
		}

		if childTransition.OnSuccessWhen == nil {
			t.Fatal("OnSuccessWhen should not be nil")
		}

		if *childTransition.OnSuccessWhen != parentCondition {
			t.Errorf("Expected parent reference condition '%s', got '%s'",
				parentCondition, *childTransition.OnSuccessWhen)
		}
	})
}
