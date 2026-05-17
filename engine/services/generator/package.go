package generator

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

// PackageInfo represents the package metadata
type PackageInfo struct {
	Name          string                 `json:"name"`
	Version       string                 `json:"version"`
	Description   string                 `json:"description"`
	URL           string                 `json:"url"`
	Documentation string                 `json:"documentation"`
	Workflows     []WorkflowInfo         `json:"workflows"`
	Transitions   []TransitionInfo       `json:"transitions"`
	Signature     string                 `json:"signature,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// WorkflowInfo represents workflow metadata
type WorkflowInfo struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"` // workflow, feature, solution
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	File        string                 `json:"file"`
	States      []string               `json:"states,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TransitionInfo represents transition metadata
type TransitionInfo struct {
	Name          string       `json:"name"`
	Service       string       `json:"service"`
	Description   string       `json:"description"`
	Methods       []MethodInfo `json:"methods"`
	File          string       `json:"file"`
	Documentation string       `json:"documentation,omitempty"`
}

// MethodInfo represents method metadata
type MethodInfo struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Arguments   []ArgInfo `json:"arguments"`
	Returns     []ArgInfo `json:"returns"`
}

// ArgInfo represents argument or return value metadata
type ArgInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// GeneratePackageInfo generates package information and saves it to a file
func GeneratePackageInfo(pkg PackageInfo, outputPath string) error {
	// Scan for workflows
	workflows, err := scanWorkflows()
	if err != nil {
		logger.Warnf("Failed to scan workflows: %s", err)
	}
	pkg.Workflows = workflows

	// Scan for transitions
	transitions, err := scanTransitions()
	if err != nil {
		logger.Warnf("Failed to scan transitions: %s", err)
	}
	pkg.Transitions = transitions

	// Add metadata
	if pkg.Metadata == nil {
		pkg.Metadata = make(map[string]interface{})
	}
	pkg.Metadata["generated_at"] = helpers.NowAsString()
	pkg.Metadata["workflow_count"] = len(pkg.Workflows)
	pkg.Metadata["transition_count"] = len(pkg.Transitions)

	// Write to file
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal package info: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write package file: %w", err)
	}

	return nil
}

// scanWorkflows scans the runtime/workflows directory for workflow files
func scanWorkflows() ([]WorkflowInfo, error) {
	var workflows []WorkflowInfo
	workflowsPath := "runtime/workflows"

	// Check if workflows directory exists
	if _, err := os.Stat(workflowsPath); os.IsNotExist(err) {
		return workflows, nil
	}

	err := filepath.Walk(workflowsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".wsl") {
			workflow := parseWorkflowFile(path)
			if workflow != nil {
				workflows = append(workflows, *workflow)
			}
		}

		return nil
	})

	return workflows, err
}

// parseWorkflowFile parses a .wsl file and extracts workflow information
func parseWorkflowFile(path string) *WorkflowInfo {
	content, err := os.ReadFile(path)
	if err != nil {
		logger.Warnf("Failed to read workflow file %s: %s", path, err)
		return nil
	}

	text := string(content)
	workflow := &WorkflowInfo{
		File:     path,
		Metadata: make(map[string]interface{}),
	}

	// Extract workflow type and name
	if strings.Contains(text, "workflow ") {
		workflow.Type = "workflow"
		workflow.Name = extractWorkflowName(text, "workflow")
	} else if strings.Contains(text, "feature ") {
		workflow.Type = "feature"
		workflow.Name = extractWorkflowName(text, "feature")
	} else if strings.Contains(text, "solution ") {
		workflow.Type = "solution"
		workflow.Name = extractWorkflowName(text, "solution")
	}

	// Extract constants
	workflow.Description = extractConstValue(text, "description")
	workflow.Version = extractConstValue(text, "version")

	// Extract states
	workflow.States = extractStates(text)

	return workflow
}

func extractWorkflowName(text, workflowType string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, workflowType+" ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func extractConstValue(text, key string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, key+":") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				value := strings.TrimSpace(parts[1])
				value = strings.Trim(value, `",`)
				return value
			}
		}
	}
	return ""
}

func extractStates(text string) []string {
	var states []string
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "state ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				stateName := strings.TrimSuffix(parts[1], "(")
				stateName = strings.TrimSpace(stateName)
				if stateName != "" && !contains(states, stateName) {
					states = append(states, stateName)
				}
			}
		}
	}
	return states
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// scanTransitions scans the modules directory for transition files
func scanTransitions() ([]TransitionInfo, error) {
	var transitions []TransitionInfo
	modulesPath := "modules"

	// Check if modules directory exists
	if _, err := os.Stat(modulesPath); os.IsNotExist(err) {
		return transitions, nil
	}

	err := filepath.Walk(modulesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".go") && strings.Contains(path, "transitions") {
			// Skip generated files
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}

			transition := parseTransitionFile(path, modulesPath)
			if transition != nil {
				transitions = append(transitions, *transition)
			}
		}

		return nil
	})

	return transitions, err
}

// parseTransitionFile parses a Go transition file and extracts metadata
func parseTransitionFile(path, modulesPath string) *TransitionInfo {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		logger.Warnf("Failed to parse transition file %s: %s", path, err)
		return nil
	}

	service, transitionName := extractServiceAndTransitionFromPath(path, modulesPath)
	transition := &TransitionInfo{
		Name:    transitionName,
		Service: service,
		File:    path,
		Methods: []MethodInfo{},
	}

	// Extract documentation from package comments
	if node.Doc != nil {
		transition.Documentation = node.Doc.Text()
	}

	// Parse methods
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name == nil || !funcDecl.Name.IsExported() {
			continue
		}

		// Skip if not a method (no receiver)
		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}

		method := MethodInfo{
			Name:      funcDecl.Name.Name,
			Arguments: []ArgInfo{},
			Returns:   []ArgInfo{},
		}

		// Extract method documentation
		if funcDecl.Doc != nil {
			method.Description = strings.TrimSpace(funcDecl.Doc.Text())
		}

		// Extract arguments
		if funcDecl.Type.Params != nil {
			for _, field := range funcDecl.Type.Params.List {
				typeStr := helpers.ExprToString(field.Type)
				if len(field.Names) > 0 {
					for _, name := range field.Names {
						method.Arguments = append(method.Arguments, ArgInfo{
							Name: name.Name,
							Type: typeStr,
						})
					}
				} else {
					method.Arguments = append(method.Arguments, ArgInfo{
						Name: fmt.Sprintf("arg%d", len(method.Arguments)),
						Type: typeStr,
					})
				}
			}
		}

		// Extract return values
		if funcDecl.Type.Results != nil {
			for _, field := range funcDecl.Type.Results.List {
				typeStr := helpers.ExprToString(field.Type)
				if len(field.Names) > 0 {
					for _, name := range field.Names {
						method.Returns = append(method.Returns, ArgInfo{
							Name: name.Name,
							Type: typeStr,
						})
					}
				} else {
					method.Returns = append(method.Returns, ArgInfo{
						Name: fmt.Sprintf("ret%d", len(method.Returns)),
						Type: typeStr,
					})
				}
			}
		}

		transition.Methods = append(transition.Methods, method)
	}

	if len(transition.Methods) > 0 {
		return transition
	}

	return nil
}

func extractServiceAndTransitionFromPath(path, modulesPath string) (string, string) {
	clean := filepath.Clean(modulesPath)
	relativePath := strings.TrimPrefix(path, clean+"/")
	parts := strings.Split(filepath.ToSlash(relativePath), "/")

	// Find "transitions" and work backward
	for i := len(parts) - 1; i >= 1; i-- {
		if parts[i] == "transitions" {
			serviceParts := parts[0:i]
			service := strings.Join(serviceParts, "/")
			transition := strings.TrimSuffix(filepath.Base(path), ".go")
			return service, transition
		}
	}
	return "unknown", strings.TrimSuffix(filepath.Base(path), ".go")
}
