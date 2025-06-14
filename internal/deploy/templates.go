package deploy

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"text/template"
	
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

//go:embed infrastructure.yaml
var embeddedTemplate string

// TemplateParams holds parameters for CloudFormation template substitution
type TemplateParams struct {
	StackName   string
}

// GetCloudFormationTemplate returns the CloudFormation template content
func GetCloudFormationTemplate(cfg *config.CLIConfig, customTemplatePath string) (string, error) {
	var templateContent string
	
	// Use custom template file if provided
	if customTemplatePath != "" {
		content, err := os.ReadFile(customTemplatePath)
		if err != nil {
			return "", fmt.Errorf("failed to read custom template file %s: %w", customTemplatePath, err)
		}
		templateContent = string(content)
	} else {
		// Use embedded template
		templateContent = embeddedTemplate
	}
	
	// Perform parameter substitution
	params := TemplateParams{
		StackName:   cfg.Deployment.StackName,
	}
	
	substitutedTemplate, err := substituteTemplateParams(templateContent, params)
	if err != nil {
		return "", fmt.Errorf("failed to substitute template parameters: %w", err)
	}
	
	return substitutedTemplate, nil
}

// substituteTemplateParams performs basic parameter substitution in the template
func substituteTemplateParams(templateContent string, params TemplateParams) (string, error) {
	tmpl, err := template.New("cloudformation").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	
	var result strings.Builder
	err = tmpl.Execute(&result, params)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	
	return result.String(), nil
}

// ValidateTemplate performs basic validation on the CloudFormation template
func ValidateTemplate(templateContent string) error {
	// Basic validation - check for required sections
	requiredSections := []string{
		"AWSTemplateFormatVersion",
		"Resources:",
	}
	
	for _, section := range requiredSections {
		if !strings.Contains(templateContent, section) {
			return fmt.Errorf("template missing required section: %s", section)
		}
	}
	
	return nil
}