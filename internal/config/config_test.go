package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRotationDefaults(t *testing.T) {
	cfg := New()
	
	expectedOverlap := 90 * time.Second
	expectedDrain := 45 * time.Second // Updated to match current normal mode config
	
	if cfg.Rotation.OverlapWindow != expectedOverlap {
		t.Errorf("Expected OverlapWindow %v, got %v", expectedOverlap, cfg.Rotation.OverlapWindow)
	}
	
	if cfg.Rotation.DrainTimeout != expectedDrain {
		t.Errorf("Expected DrainTimeout %v, got %v", expectedDrain, cfg.Rotation.DrainTimeout)
	}
}

func TestDefaultCLIConfig(t *testing.T) {
	cfg := DefaultCLIConfig()
	
	// Test AWS defaults
	if cfg.AWS.Region != "us-west-2" {
		t.Errorf("Expected default region us-west-2, got %s", cfg.AWS.Region)
	}
	
	// Test deployment defaults
	if !strings.HasPrefix(cfg.Deployment.StackName, "lambda-nat-proxy-") {
		t.Errorf("Expected default stack name to start with lambda-nat-proxy-, got %s", cfg.Deployment.StackName)
	}
	if cfg.Deployment.Mode != ModeNormal {
		t.Errorf("Expected default mode normal, got %s", cfg.Deployment.Mode)
	}
	
	// Test proxy defaults
	if cfg.Proxy.Port != 1080 {
		t.Errorf("Expected default port 1080, got %d", cfg.Proxy.Port)
	}
	if cfg.Proxy.STUNServer == "" {
		t.Error("Expected STUN server to be set by default")
	}
}

func TestLoadCLIConfig(t *testing.T) {
	// Test loading with no config file (should use defaults)
	cfg, err := LoadCLIConfig("")
	if err != nil {
		t.Fatalf("Expected no error loading default config, got %v", err)
	}
	
	// Check that defaults are loaded
	if cfg.AWS.Region != "us-west-2" {
		t.Errorf("Expected default region us-west-2, got %s", cfg.AWS.Region)
	}
	if cfg.Proxy.Port != 1080 {
		t.Errorf("Expected default port 1080, got %d", cfg.Proxy.Port)
	}
}

func TestLoadCLIConfigWithFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test-config.yaml")
	
	configContent := `aws:
  region: "us-east-1"
  profile: "test-profile"
deployment:
  stack_name: "test-stack"
  mode: "performance"
proxy:
  port: 9090
`
	
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}
	
	// Load config with specific file
	cfg, err := LoadCLIConfig(configFile)
	if err != nil {
		t.Fatalf("Expected no error loading config file, got %v", err)
	}
	
	// Verify custom values were loaded
	if cfg.AWS.Region != "us-east-1" {
		t.Errorf("Expected region us-east-1, got %s", cfg.AWS.Region)
	}
	if cfg.AWS.Profile != "test-profile" {
		t.Errorf("Expected profile test-profile, got %s", cfg.AWS.Profile)
	}
	if cfg.Deployment.StackName != "test-stack" {
		t.Errorf("Expected stack name test-stack, got %s", cfg.Deployment.StackName)
	}
	if cfg.Deployment.Mode != ModePerformance {
		t.Errorf("Expected mode performance, got %s", cfg.Deployment.Mode)
	}
	if cfg.Proxy.Port != 9090 {
		t.Errorf("Expected port 9090, got %d", cfg.Proxy.Port)
	}
}

func TestValidateCLIConfig(t *testing.T) {
	// Test valid config
	validCfg := &CLIConfig{
		AWS: AWSConfig{
			Region: "us-west-2",
		},
		Deployment: DeploymentConfig{
			StackName: "test-stack",
			Mode:      ModeNormal,
		},
		Proxy: ProxyConfig{
			Port:       8080,
			STUNServer: "stun.l.google.com:19302",
		},
	}
	
	if err := ValidateCLIConfig(validCfg); err != nil {
		t.Errorf("Expected no error for valid config, got %v", err)
	}
	
	// Test invalid config - empty region
	invalidCfg := &CLIConfig{
		AWS: AWSConfig{
			Region: "",
		},
		Deployment: DeploymentConfig{
			StackName: "test-stack",
			Mode:      ModeNormal,
		},
		Proxy: ProxyConfig{
			Port:       8080,
			STUNServer: "stun.l.google.com:19302",
		},
	}
	
	if err := ValidateCLIConfig(invalidCfg); err == nil {
		t.Error("Expected error for config with empty region")
	}
	
	// Test invalid mode
	invalidModeCfg := &CLIConfig{
		AWS: AWSConfig{
			Region: "us-west-2",
		},
		Deployment: DeploymentConfig{
			StackName: "test-stack",
			Mode:      "invalid",
		},
		Proxy: ProxyConfig{
			Port:       8080,
			STUNServer: "stun.l.google.com:19302",
		},
	}
	
	if err := ValidateCLIConfig(invalidModeCfg); err == nil {
		t.Error("Expected error for config with invalid mode")
	}
}