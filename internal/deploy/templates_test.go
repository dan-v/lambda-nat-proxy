package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

func TestGetCloudFormationTemplate(t *testing.T) {
	cfg := &config.CLIConfig{
		Deployment: config.DeploymentConfig{
			StackName: "test-stack",
		},
	}
	
	template, err := GetCloudFormationTemplate(cfg, "")
	if err != nil {
		t.Fatalf("Expected no error getting template, got %v", err)
	}
	
	if template == "" {
		t.Error("Expected template content, got empty string")
	}
	
	// Check that template contains expected CloudFormation content
	if !strings.Contains(template, "AWSTemplateFormatVersion") {
		t.Error("Expected template to contain AWSTemplateFormatVersion")
	}
}

func TestGetCloudFormationTemplateWithCustomFile(t *testing.T) {
	cfg := &config.CLIConfig{
		Deployment: config.DeploymentConfig{
			StackName: "test-stack",
		},
	}
	
	// Create a temporary custom template file
	tempDir := t.TempDir()
	customTemplatePath := filepath.Join(tempDir, "custom.yaml")
	
	customContent := `AWSTemplateFormatVersion: '2010-09-09'
Description: Custom test template for {{.StackName}}
Resources:
  TestResource:
    Type: AWS::S3::Bucket
`
	
	err := os.WriteFile(customTemplatePath, []byte(customContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create custom template file: %v", err)
	}
	
	template, err := GetCloudFormationTemplate(cfg, customTemplatePath)
	if err != nil {
		t.Fatalf("Expected no error getting custom template, got %v", err)
	}
	
	// Check that parameter substitution worked
	if !strings.Contains(template, "test-stack") {
		t.Error("Expected template to contain substituted stack name")
	}
}

func TestGetCloudFormationTemplateWithMissingFile(t *testing.T) {
	cfg := &config.CLIConfig{
		Deployment: config.DeploymentConfig{
			StackName: "test-stack",
		},
	}
	
	_, err := GetCloudFormationTemplate(cfg, "nonexistent.yaml")
	if err == nil {
		t.Error("Expected error for missing template file")
	}
}

func TestValidateTemplate(t *testing.T) {
	validTemplate := `AWSTemplateFormatVersion: '2010-09-09'
Description: Test template
Resources:
  TestBucket:
    Type: AWS::S3::Bucket
`
	
	err := ValidateTemplate(validTemplate)
	if err != nil {
		t.Errorf("Expected no error for valid template, got %v", err)
	}
	
	invalidTemplate := `Description: Missing AWSTemplateFormatVersion and Resources`
	
	err = ValidateTemplate(invalidTemplate)
	if err == nil {
		t.Error("Expected error for invalid template")
	}
}

func TestSubstituteTemplateParams(t *testing.T) {
	templateContent := `Stack: {{.StackName}}
`
	
	params := TemplateParams{
		StackName: "my-stack",
	}
	
	result, err := substituteTemplateParams(templateContent, params)
	if err != nil {
		t.Fatalf("Expected no error substituting params, got %v", err)
	}
	
	if !strings.Contains(result, "my-stack") {
		t.Error("Expected result to contain stack name")
	}
}