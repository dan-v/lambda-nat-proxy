package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCLICommands tests the main CLI commands of lambda-nat-proxy
func TestCLICommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Build the lambda-nat-proxy binary first
	if err := buildLambdaProxy(); err != nil {
		t.Fatalf("Failed to build lambda-nat-proxy: %v", err)
	}

	t.Run("Version", testVersionCommand)
	t.Run("Config", testConfigCommands)
	t.Run("Deploy", testDeployCommand)
	t.Run("Status", testStatusCommand)
	t.Run("Destroy", testDestroyCommand)
}

// buildLambdaProxy builds the lambda-nat-proxy binary for testing
func buildLambdaProxy() error {
	cmd := exec.Command("make", "build")
	cmd.Dir = "../.." // From cmd/lambda-nat-proxy back to root
	return cmd.Run()
}

// testVersionCommand tests the version command
func testVersionCommand(t *testing.T) {
	cmd := exec.Command("../../build/lambda-nat-proxy", "version")
	cmd.Dir = "."
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Version command failed: %v\nOutput: %s", err, output)
	}
	
	outputStr := string(output)
	if !strings.Contains(outputStr, "lambda-nat-proxy") {
		t.Errorf("Version output should contain 'lambda-nat-proxy', got: %s", outputStr)
	}
	
	t.Logf("Version command output: %s", outputStr)
}

// testConfigCommands tests config-related commands
func testConfigCommands(t *testing.T) {
	// Create a temporary directory for config tests
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "lambda-nat-proxy.yaml")
	
	// Test config init
	t.Run("Init", func(t *testing.T) {
		// Get absolute path to binary
		binPath, err := filepath.Abs("../../build/lambda-nat-proxy")
		if err != nil {
			t.Fatalf("Failed to get absolute path to binary: %v", err)
		}
		
		cmd := exec.Command(binPath, "config", "init", "--output", configPath)
		cmd.Dir = tempDir // Run in temp directory so file is created there
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Config init failed: %v\nOutput: %s", err, output)
		}
		
		// Check that config file was created in the working directory
		defaultConfigPath := filepath.Join(tempDir, "lambda-nat-proxy.yaml")
		if _, err := os.Stat(defaultConfigPath); os.IsNotExist(err) {
			t.Errorf("Config file was not created at %s", defaultConfigPath)
		}
		
		// Update configPath for subsequent tests
		configPath = defaultConfigPath
		
		t.Logf("Config init output: %s", output)
	})
	
	// Test config show
	t.Run("Show", func(t *testing.T) {
		cmd := exec.Command("../../build/lambda-nat-proxy", "config", "show", "--config", configPath)
		cmd.Dir = "."
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Config show failed: %v\nOutput: %s", err, output)
		}
		
		outputStr := string(output)
		if !strings.Contains(outputStr, "aws:") {
			t.Errorf("Config show should contain AWS config, got: %s", outputStr)
		}
		
		t.Logf("Config show output: %s", outputStr)
	})
}

// testDeployCommand tests the deploy command
func testDeployCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "../../build/lambda-nat-proxy", "deploy", "--dry-run")
	cmd.Dir = "."
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Deploy dry-run failed: %v\nOutput: %s", err, output)
	}
	
	outputStr := string(output)
	expectedStrings := []string{
		"Dry run",
		"Stack Name:",
		"AWS Region:",
		"Performance Mode:",
		"Lambda Memory:",
		"Lambda Timeout:",
	}
	
	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Deploy dry-run output missing '%s', got: %s", expected, outputStr)
		}
	}
	
	t.Logf("Deploy dry-run output: %s", outputStr)
}

// testStatusCommand tests the status command against existing deployment
func testStatusCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	
	// Test status command
	cmd := exec.CommandContext(ctx, "../../build/lambda-nat-proxy", "status")
	cmd.Dir = "."
	
	output, err := cmd.CombinedOutput()
	outputStr := string(output)
	
	// Skip test if AWS credentials are not available
	if err != nil && strings.Contains(outputStr, "invalid AWS credentials") {
		t.Skip("Skipping status test - AWS credentials not available")
		return
	}
	
	if err != nil {
		t.Fatalf("Status command failed: %v\nOutput: %s", err, output)
	}
	expectedStrings := []string{
		"Lambda NAT Proxy Status",
		"Overall Status:",
		"CloudFormation Stack",
		"Lambda Function",
		"S3 Bucket",
		"Quick Status",
	}
	
	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Status output missing '%s', got: %s", expected, outputStr)
		}
	}
	
	t.Logf("Status command output: %s", outputStr)
	
	// Test JSON format
	t.Run("JSONFormat", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "../../build/lambda-nat-proxy", "status", "--format", "json")
		cmd.Dir = "."
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Status JSON command failed: %v\nOutput: %s", err, output)
		}
		
		outputStr := string(output)
		if !strings.Contains(outputStr, "{") || !strings.Contains(outputStr, "}") {
			t.Errorf("Status JSON output should be valid JSON, got: %s", outputStr)
		}
		
		t.Logf("Status JSON output: %s", outputStr)
	})
	
	// Test YAML format
	t.Run("YAMLFormat", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "../../build/lambda-nat-proxy", "status", "--format", "yaml")
		cmd.Dir = "."
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Status YAML command failed: %v\nOutput: %s", err, output)
		}
		
		outputStr := string(output)
		if !strings.Contains(outputStr, "summary:") {
			t.Errorf("Status YAML output should contain YAML keys, got: %s", outputStr)
		}
		
		t.Logf("Status YAML output: %s", outputStr)
	})
}

// testDestroyCommand tests the destroy command with cancellation
func testDestroyCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Test destroy command with cancellation
	cmd := exec.CommandContext(ctx, "../../build/lambda-nat-proxy", "destroy")
	cmd.Dir = "."
	
	// Send "no" to cancel the destruction
	cmd.Stdin = strings.NewReader("no\n")
	
	output, _ := cmd.CombinedOutput()
	// The command should exit with non-zero status when cancelled, but that's expected
	
	outputStr := string(output)
	
	// Skip test if AWS credentials are not available
	if strings.Contains(outputStr, "invalid AWS credentials") {
		t.Skip("Skipping destroy test - AWS credentials not available")
		return
	}
	
	expectedStrings := []string{
		"Lambda NAT Proxy Destruction Plan",
		"The following resources will be PERMANENTLY DELETED:",
		"CloudFormation Stack:",
		"Lambda Function:",
		"CloudWatch Logs:",
		"WARNING: This action cannot be undone!",
	}
	
	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Destroy output missing '%s', got: %s", expected, outputStr)
		}
	}
	
	// Should show cancellation message
	if !strings.Contains(outputStr, "cancelled") {
		t.Errorf("Destroy should show cancellation message, got: %s", outputStr)
	}
	
	t.Logf("Destroy command output: %s", outputStr)
}

// TestCommandHelp tests that all commands show proper help
func TestCommandHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping help tests in short mode")
	}
	
	// Build the lambda-nat-proxy binary first
	if err := buildLambdaProxy(); err != nil {
		t.Fatalf("Failed to build lambda-nat-proxy: %v", err)
	}
	
	commands := []string{
		"",           // root command
		"run",
		"deploy", 
		"destroy",
		"status",
		"config",
		"version",
	}
	
	for _, command := range commands {
		t.Run("Help_"+command, func(t *testing.T) {
			var cmd *exec.Cmd
			if command == "" {
				cmd = exec.Command("../../build/lambda-nat-proxy", "--help")
			} else {
				cmd = exec.Command("../../build/lambda-nat-proxy", command, "--help")
			}
			cmd.Dir = "."
			
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Help command failed for %s: %v\nOutput: %s", command, err, output)
			}
			
			outputStr := string(output)
			if !strings.Contains(outputStr, "Usage:") {
				t.Errorf("Help for %s should contain Usage, got: %s", command, outputStr)
			}
			
			t.Logf("Help for %s: %s", command, outputStr)
		})
	}
}

// TestConfigValidation tests configuration validation
func TestConfigValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping config validation tests in short mode")
	}
	
	// Build the lambda-nat-proxy binary first
	if err := buildLambdaProxy(); err != nil {
		t.Fatalf("Failed to build lambda-nat-proxy: %v", err)
	}
	
	// Create a temporary directory for config tests
	tempDir := t.TempDir()
	
	// Test with invalid config
	invalidConfigPath := filepath.Join(tempDir, "invalid.yaml")
	invalidConfig := `
aws:
  region: ""  # Invalid empty region
deployment:
  mode: "invalid-mode"  # Invalid mode
`
	
	if err := os.WriteFile(invalidConfigPath, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}
	
	// Test that commands fail with invalid config
	cmd := exec.Command("../../build/lambda-nat-proxy", "status", "--config", invalidConfigPath)
	cmd.Dir = "."
	
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("Command should fail with invalid config, but succeeded")
	}
	
	outputStr := string(output)
	if !strings.Contains(outputStr, "validation") {
		t.Errorf("Error should mention validation, got: %s", outputStr)
	}
	
	t.Logf("Config validation error output: %s", outputStr)
}