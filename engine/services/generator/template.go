package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kuetix/helpers"
)

// GenerateTemplate generates a template file based on the type
func GenerateTemplate(templateType, name, outputPath string) error {
	var content string
	var filename string

	switch templateType {
	case "solution":
		content = generateSolutionTemplate(name)
		filename = fmt.Sprintf("%s_solution.wsl", helpers.ToSnakeCase(name))
	case "feature":
		content = generateFeatureTemplate(name)
		filename = fmt.Sprintf("%s_feature.wsl", helpers.ToSnakeCase(name))
	case "workflow":
		content = generateWorkflowTemplate(name)
		filename = fmt.Sprintf("%s.wsl", helpers.ToSnakeCase(name))
	case "swsl-solution":
		content = generateSWSLSolutionTemplate(name)
		filename = fmt.Sprintf("%s_solution.swsl", helpers.ToSnakeCase(name))
	case "swsl-feature":
		content = generateSWSLFeatureTemplate(name)
		filename = fmt.Sprintf("%s_feature.swsl", helpers.ToSnakeCase(name))
	case "swsl-workflow", "swsl":
		content = generateSWSLWorkflowTemplate(name)
		filename = fmt.Sprintf("%s.swsl", helpers.ToSnakeCase(name))
	case "transition":
		content = generateTransitionTemplate(name)
		filename = fmt.Sprintf("%s.go", helpers.ToSnakeCase(name))
	default:
		return fmt.Errorf("unknown template type: %s", templateType)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fullPath := filepath.Join(outputPath, filename)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write template file: %w", err)
	}

	fmt.Printf("Generated %s template at: %s\n", templateType, fullPath)
	return nil
}

func generateSolutionTemplate(name string) string {
	solutionName := helpers.ToSnakeCase(name)
	return fmt.Sprintf(`module %s

import services/common

const {
    event: "%s",
    description: "Solution for %s",
    version: "1.0.0",
    enabled: true
}

solution %s {
  start: InitialState

  state InitialState {
    action services/common/response.ResponseValue(value: "Solution initialized", statusCode: 200)
    end ok
  }
}
`, solutionName, name, name, solutionName)
}

func generateFeatureTemplate(name string) string {
	featureName := helpers.ToSnakeCase(name)
	return fmt.Sprintf(`module %s

import services/common

const {
    event: "%s",
    description: "Feature for %s",
    version: "1.0.0",
    enabled: true
}

feature %s {
  start: InitialState

  state InitialState(input) {
    action services/common/response.ResponseValue(value: $input, statusCode: 200) as Result
    on success -> FinalState
  }

  state FinalState(Result) {
    action services/common/response.ResponseValue(value: $Result.message, statusCode: 200)
    end ok
  }
}
`, featureName, name, name, featureName)
}

func generateWorkflowTemplate(name string) string {
	workflowName := helpers.ToSnakeCase(name)
	return fmt.Sprintf(`module %s

import services/common

const {
    event: "%s",
    description: "Workflow for %s",
    version: "1.0.0",
    enabled: true
}

workflow %s {
  start: InitialState

  state InitialState {
    action services/common/response.ResponseValue(value: "Workflow started", statusCode: 200) as Result
    on success -> ProcessState
  }

  state ProcessState {
    action services/common/response.ResponseValue(value: "Processing...", statusCode: 200) as ProcessResult
    on success -> FinalState
  }

  state FinalState {
    action services/common/response.ResponseValue(value: "Workflow completed", statusCode: 200)
    end ok
  }
}
`, workflowName, name, name, workflowName)
}

func generateTransitionTemplate(name string) string {
	transitionName := helpers.ToSnakeCase(name)
	structName := helpers.ToCamelCase(name) + "Transitions"
	constructorName := "New" + helpers.ToCamelCase(name) + "Transitions"

	return fmt.Sprintf(`package transitions

import (
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

// %s handles %s operations
type %s struct {
	workflow.BaseServiceTransition
}

// %s creates a new instance of %s
func %s() interfaces.ServiceTransitions {
	return &%s{}
}

// Execute performs the main %s operation
func (t *%s) Execute(input string) (r domain.FlowStepResult) {
	// TODO: Implement your logic here
	
	r.Success = true
	r.StatusCode = 200
	r.Response = map[string]interface{}{
		"message": "Operation executed successfully",
		"input":   input,
	}
	return
}

// Process processes data for %s
func (t *%s) Process(data interface{}) (r domain.FlowStepResult) {
	// TODO: Implement your processing logic here
	
	r.Success = true
	r.StatusCode = 200
	r.Response = data
	return
}
`, structName, transitionName, structName, constructorName, structName, constructorName, structName, transitionName, structName, transitionName, structName)
}

// generateSWSLSolutionTemplate generates a SimplifiedWSL solution template
func generateSWSLSolutionTemplate(name string) string {
	solutionName := helpers.ToSnakeCase(name)
	return fmt.Sprintf(`module %s

solution %s

const {
    event: "%s",
    description: "SimplifiedWSL solution for %s",
    version: "1.0.0"
}

def errors.LogError(msg: "Error occurred") as errorHandler -> .

// Initialize solution
services/common.ResponseValue(value: "Solution initialized", statusCode: 200) as result <- errorHandler ->

// Call features or workflows as needed
// Example: feature:my_feature() ->
// Example: workflow:my_workflow() ->

// Complete
services/common.ResponseValue(value: "Solution completed", statusCode: 200) -> .
`, solutionName, solutionName, name, name)
}

// generateSWSLFeatureTemplate generates a SimplifiedWSL feature template
func generateSWSLFeatureTemplate(name string) string {
	featureName := helpers.ToSnakeCase(name)
	return fmt.Sprintf(`module %s

feature %s

const {
    event: "%s",
    description: "SimplifiedWSL feature for %s",
    version: "1.0.0"
}

def errors.LogError(msg: "Error occurred") as errorHandler -> .

// Initialize feature
services/common.ResponseValue(value: "Feature initialized", statusCode: 200) as result <- errorHandler ->

// Call workflows as needed
// Example: workflow:my_workflow() ->

// Process result
services/common.ResponseValue(value: $result.message, statusCode: 200) as processed <- errorHandler ->

// Complete
services/common.ResponseValue(value: "Feature completed", statusCode: 200) -> .
`, featureName, featureName, name, name)
}

// generateSWSLWorkflowTemplate generates a SimplifiedWSL workflow template
func generateSWSLWorkflowTemplate(name string) string {
	workflowName := helpers.ToSnakeCase(name)
	return fmt.Sprintf(`module %s

workflow %s

const {
    event: "%s",
    description: "SimplifiedWSL workflow for %s",
    version: "1.0.0"
}

def errors.LogError(msg: "Error occurred") as errorHandler -> .

// Start workflow
services/common.ResponseValue(value: "Workflow started", statusCode: 200) as startResult <- errorHandler ->

// Process
services/common.ResponseValue(value: "Processing...", statusCode: 200) as processResult <- errorHandler ->

// Complete
services/common.ResponseValue(value: "Workflow completed", statusCode: 200) -> .
`, workflowName, workflowName, name, name)
}
