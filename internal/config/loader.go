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
	v.SetConfigType("yaml")
	
	// Determine config file to use
	var foundConfig bool
	if configPath != "" {
		// Use specific config file path if provided
		v.SetConfigFile(configPath)
		foundConfig = true
	} else {
		// Search for specific config files (not just any file named lambda-nat-proxy)
		configFiles := []string{
			"./lambda-nat-proxy.yaml",                                           // Current directory
			"./lambda-nat-proxy.yml",                                            // Current directory (alt extension)
			filepath.Join(xdg.ConfigHome, "lambda-nat-proxy", "lambda-nat-proxy.yaml"),      // User config
			filepath.Join(xdg.ConfigHome, "lambda-nat-proxy", "lambda-nat-proxy.yml"),       // User config (alt)
			"/etc/lambda-nat-proxy/lambda-nat-proxy.yaml",                      // System directory
			"/etc/lambda-nat-proxy/lambda-nat-proxy.yml",                       // System directory (alt)
		}
		
		// Also check XDG config dirs
		for _, dir := range xdg.ConfigDirs {
			configFiles = append(configFiles,
				filepath.Join(dir, "lambda-nat-proxy", "lambda-nat-proxy.yaml"),
				filepath.Join(dir, "lambda-nat-proxy", "lambda-nat-proxy.yml"),
			)
		}
		
		// Find the first existing config file
		for _, configFile := range configFiles {
			if _, err := os.Stat(configFile); err == nil {
				v.SetConfigFile(configFile)
				foundConfig = true
				break
			}
		}
	}
	
	// Try to read config file (only if one was found)
	if foundConfig {
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("error reading config file: %w", err)
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