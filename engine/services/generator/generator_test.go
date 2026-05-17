package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateTemplate(t *testing.T) {
	tests := []struct {
		name         string
		templateType string
		templateName string
		wantErr      bool
		checkContent string
	}{
		{
			name:         "Generate workflow template",
			templateType: "workflow",
			templateName: "TestWorkflow",
			wantErr:      false,
			checkContent: "workflow test_workflow",
		},
		{
			name:         "Generate feature template",
			templateType: "feature",
			templateName: "TestFeature",
			wantErr:      false,
			checkContent: "feature test_feature",
		},
		{
			name:         "Generate solution template",
			templateType: "solution",
			templateName: "TestSolution",
			wantErr:      false,
			checkContent: "solution test_solution",
		},
		{
			name:         "Generate transition template",
			templateType: "transition",
			templateName: "TestTransition",
			wantErr:      false,
			checkContent: "type TesttransitionTransitions struct",
		},
		{
			name:         "Generate SWSL workflow template",
			templateType: "swsl-workflow",
			templateName: "TestWorkflow",
			wantErr:      false,
			checkContent: "workflow test_workflow",
		},
		{
			name:         "Generate SWSL feature template",
			templateType: "swsl-feature",
			templateName: "TestFeature",
			wantErr:      false,
			checkContent: "feature test_feature",
		},
		{
			name:         "Generate SWSL solution template",
			templateType: "swsl-solution",
			templateName: "TestSolution",
			wantErr:      false,
			checkContent: "solution test_solution",
		},
		{
			name:         "Generate SWSL template with alias",
			templateType: "swsl",
			templateName: "TestSWSL",
			wantErr:      false,
			checkContent: "workflow test_s_w_s_l",
		},
		{
			name:         "Invalid template type",
			templateType: "invalid",
			templateName: "Test",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temp directory for testing
			tmpDir := t.TempDir()

			err := GenerateTemplate(tt.templateType, tt.templateName, tmpDir)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // No need to check content if we expected an error
			}

			// Check if file was created
			files, err := os.ReadDir(tmpDir)
			if err != nil {
				t.Fatalf("Failed to read temp directory: %v", err)
			}

			if len(files) == 0 {
				t.Fatal("No file was created")
			}

			// Read the generated file
			content, err := os.ReadFile(filepath.Join(tmpDir, files[0].Name()))
			if err != nil {
				t.Fatalf("Failed to read generated file: %v", err)
			}

			// Check if content contains expected string
			if !strings.Contains(string(content), tt.checkContent) {
				t.Errorf("Generated content does not contain expected string: %s", tt.checkContent)
			}
		})
	}
}

func TestSignPackage(t *testing.T) {
	// Create a temporary package file
	tmpDir := t.TempDir()
	packageFile := filepath.Join(tmpDir, "test_kuetix.json")

	packageContent := `{
  "name": "test-package",
  "version": "1.0.0",
  "description": "Test package",
  "workflows": [],
  "transitions": []
}`

	if err := os.WriteFile(packageFile, []byte(packageContent), 0644); err != nil {
		t.Fatalf("Failed to create test package file: %v", err)
	}

	// Test signing
	apiToken := "test-token-12345"
	err := SignPackage(packageFile, apiToken)
	if err != nil {
		t.Fatalf("SignPackage() failed: %v", err)
	}

	// Read the signed package
	content, err := os.ReadFile(packageFile)
	if err != nil {
		t.Fatalf("Failed to read signed package: %v", err)
	}

	// Check if signature was added
	if !strings.Contains(string(content), `"signature"`) {
		t.Error("Signature was not added to the package")
	}
}

func TestGeneratePackageInfo(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "kuetix.json")

	pkg := PackageInfo{
		Name:        "test-package",
		Version:     "1.0.0",
		Description: "Test package description",
	}

	err := GeneratePackageInfo(pkg, outputPath)
	if err != nil {
		t.Fatalf("GeneratePackageInfo() failed: %v", err)
	}

	// Check if file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("Package file was not created")
	}

	// Read the file
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read package file: %v", err)
	}

	// Check if content contains expected fields
	expectedFields := []string{
		`"name": "test-package"`,
		`"version": "1.0.0"`,
		`"description": "Test package description"`,
		`"workflows"`,
		`"transitions"`,
		`"metadata"`,
	}

	for _, field := range expectedFields {
		if !strings.Contains(string(content), field) {
			t.Errorf("Package file does not contain expected field: %s", field)
		}
	}
}
