package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

// LoadCLIConfig loads configuration from files, environment, and returns a merged config
func LoadCLIConfig(configPath string) (*CLIConfig, error) {
	cfg := DefaultCLIConfig()
	
	// Initialize viper
	v := viper.New()
	v.SetConfigName("lambda-nat-proxy")
	v.SetConfigType("yaml")
	
	// Add search paths
	if configPath != "" {
		// Use specific config file path if provided
		v.SetConfigFile(configPath)
	} else {
		// Search in XDG-compliant locations
		v.AddConfigPath(".")                                                    // Current directory
		v.AddConfigPath(filepath.Join(xdg.ConfigHome, "lambda-nat-proxy"))      // User config (~/.config/lambda-nat-proxy)
		v.AddConfigPath("/etc/lambda-nat-proxy")                               // System directory
		
		// Also check XDG config dirs
		for _, dir := range xdg.ConfigDirs {
			v.AddConfigPath(filepath.Join(dir, "lambda-nat-proxy"))
		}
	}
	
	// Set environment variable prefix
	v.SetEnvPrefix("LAMBDA_PROXY")
	v.AutomaticEnv()
	
	// Map environment variables to config keys
	v.BindEnv("aws.region", "AWS_REGION")
	v.BindEnv("aws.profile", "AWS_PROFILE")
	v.BindEnv("deployment.mode", "MODE")
	v.BindEnv("proxy.port", "SOCKS5_PORT")
	
	// Try to read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file was found but another error was produced
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, continue with defaults and env vars
	}
	
	// Unmarshal into our config struct
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}
	
	return cfg, nil
}

// WriteExampleConfig creates an example configuration file
func WriteExampleConfig(filePath string) error {
	exampleConfig := `# Lambda NAT Proxy Configuration File
# This file contains all available configuration options with their default values

# AWS Configuration
aws:
  region: "us-west-2"           # AWS region to use
  profile: ""                   # AWS profile (leave empty for default credential chain)

# Deployment Configuration  
deployment:
  stack_name: "lambda-nat-proxy-a1b2c3d4"  # CloudFormation stack name (unique suffix auto-generated)
  mode: "normal"                # Performance mode: test, normal, performance

# Proxy Configuration
proxy:
  port: 1080                    # SOCKS5 proxy port (standard SOCKS port)
  stun_server: "stun.l.google.com:19302"  # STUN server for NAT traversal
`
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	
	// Write the example config
	if err := os.WriteFile(filePath, []byte(exampleConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", filePath, err)
	}
	
	return nil
}

// FindConfigFile searches for a config file in XDG-compliant locations
func FindConfigFile() (string, error) {
	searchPaths := []string{
		"lambda-nat-proxy.yaml",
		"lambda-nat-proxy.yml",
		filepath.Join(xdg.ConfigHome, "lambda-nat-proxy", "lambda-nat-proxy.yaml"),
		filepath.Join(xdg.ConfigHome, "lambda-nat-proxy", "lambda-nat-proxy.yml"),
		"/etc/lambda-nat-proxy/lambda-nat-proxy.yaml",
		"/etc/lambda-nat-proxy/lambda-nat-proxy.yml",
	}
	
	// Also check XDG config dirs
	for _, dir := range xdg.ConfigDirs {
		searchPaths = append(searchPaths, 
			filepath.Join(dir, "lambda-nat-proxy", "lambda-nat-proxy.yaml"),
			filepath.Join(dir, "lambda-nat-proxy", "lambda-nat-proxy.yml"),
		)
	}
	
	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	
	return "", fmt.Errorf("no config file found in standard locations")
}

// GetDefaultConfigPath returns the default path for creating a new config file
func GetDefaultConfigPath() string {
	return filepath.Join(xdg.ConfigHome, "lambda-nat-proxy", "lambda-nat-proxy.yaml")
}

// GetConfigSource returns information about where config values came from
func GetConfigSource(v *viper.Viper, key string) string {
	// Check if value was set via command line flag
	if v.IsSet(key) {
		// This is a simplified check - in reality you'd need to track
		// the source more precisely
		return "file/env"
	}
	return "default"
}