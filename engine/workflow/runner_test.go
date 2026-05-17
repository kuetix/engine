package workflow

import (
	"testing"

	"github.com/kuetix/engine/engine/domain"
)

// TestRunnerFactory tests the runner factory creates appropriate runners
func TestRunnerFactory(t *testing.T) {
	config := domain.WorkflowConfigItem{
		Name:   "test",
		Amount: 1,
		Retry:  1,
	}
	app := domain.Application{
		EngineName: "test-engine",
	}

	factory := NewRunnerFactory(config, app)

	tests := []struct {
		flowType     string
		expectedType string
	}{
		{"workflow", "workflow"},
		{"feature", "feature"},
		{"solution", "solution"},
		{"", "workflow"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.flowType, func(t *testing.T) {
			runner, err := factory.CreateRunner(tt.flowType)
			if err != nil {
				t.Fatalf("Failed to create runner for type %s: %v", tt.flowType, err)
			}

			if runner.GetType() != tt.expectedType {
				t.Errorf("Expected runner type %s, got %s", tt.expectedType, runner.GetType())
			}

			if err := runner.Validate(); err != nil {
				t.Errorf("Runner validation failed: %v", err)
			}
		})
	}
}

// TestWorkflowRunner tests the workflow runner
func TestWorkflowRunner(t *testing.T) {
	config := domain.WorkflowConfigItem{
		Name:   "test_workflow",
		Amount: 1,
		Retry:  1,
	}
	app := domain.Application{
		EngineName: "test-engine",
	}

	runner := NewWorkflowRunner(config, app)

	if runner.GetType() != "workflow" {
		t.Errorf("Expected type 'workflow', got %s", runner.GetType())
	}

	if err := runner.Validate(); err != nil {
		t.Errorf("Validation failed: %v", err)
	}
}

// TestFeatureRunner tests the feature runner
func TestFeatureRunner(t *testing.T) {
	config := domain.WorkflowConfigItem{
		Name:   "test_feature",
		Amount: 1,
		Retry:  1,
	}
	app := domain.Application{
		EngineName: "test-engine",
	}

	runner := NewFeatureRunner(config, app)

	if runner.GetType() != "feature" {
		t.Errorf("Expected type 'feature', got %s", runner.GetType())
	}

	if err := runner.Validate(); err != nil {
		t.Errorf("Validation failed: %v", err)
	}
}

// TestSolutionRunner tests the solution runner
func TestSolutionRunner(t *testing.T) {
	config := domain.WorkflowConfigItem{
		Name:   "test_solution",
		Amount: 1,
		Retry:  1,
	}
	app := domain.Application{
		EngineName: "test-engine",
	}

	runner := NewSolutionRunner(config, app)

	if runner.GetType() != "solution" {
		t.Errorf("Expected type 'solution', got %s", runner.GetType())
	}

	if err := runner.Validate(); err != nil {
		t.Errorf("Validation failed: %v", err)
	}
}

// TestRunnerRegistry tests custom runner registration
func TestRunnerRegistry(t *testing.T) {
	config := domain.WorkflowConfigItem{
		Name:   "test_custom",
		Amount: 1,
		Retry:  1,
	}
	app := domain.Application{
		EngineName: "test-engine",
	}

	// Create a custom runner
	//goland:noinspection GoUnusedType
	type customRunner struct {
		config      domain.WorkflowConfigItem
		application domain.Application
	}

	customRunnerFactory := func(cfg domain.WorkflowConfigItem, a domain.Application) Runner {
		return &WorkflowRunner{config: cfg, application: a} // Use WorkflowRunner as base
	}

	// Register custom runner
	RegisterCustomRunner("custom_type", customRunnerFactory)

	// Try to create it
	factory := NewRunnerFactory(config, app)
	runner, err := factory.CreateRunnerWithRegistry("custom_type")
	if err != nil {
		t.Fatalf("Failed to create custom runner: %v", err)
	}

	if runner == nil {
		t.Fatal("Custom runner is nil")
	}
}

// TestRunnerValidation tests runner validation
func TestRunnerValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      domain.WorkflowConfigItem
		expectError bool
	}{
		{
			name: "valid config",
			config: domain.WorkflowConfigItem{
				Name:   "test",
				Amount: 1,
				Retry:  1,
			},
			expectError: false,
		},
		{
			name: "empty name",
			config: domain.WorkflowConfigItem{
				Name:   "",
				Amount: 1,
				Retry:  1,
			},
			expectError: true,
		},
	}

	app := domain.Application{
		EngineName: "test-engine",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewWorkflowRunner(tt.config, app)
			err := runner.Validate()

			if tt.expectError && err == nil {
				t.Error("Expected validation error, got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error, got: %v", err)
			}
		})
	}
}

// TestExecuteWithRunner tests execution with runner factory
func TestExecuteWithRunner(t *testing.T) {
	// Skip this test as it requires a full environment setup
	t.Skip("Skipping ExecuteWithRunner test - requires full environment setup")
}
