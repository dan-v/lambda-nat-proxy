package config

import (
	"time"
)

// CLIConfig represents the complete configuration for lambda-nat-proxy CLI
type CLIConfig struct {
	// AWS configuration
	AWS AWSConfig `yaml:"aws" json:"aws"`
	
	// Deployment configuration  
	Deployment DeploymentConfig `yaml:"deployment" json:"deployment"`
	
	// Proxy configuration
	Proxy ProxyConfig `yaml:"proxy" json:"proxy"`
}

// AWSConfig holds AWS-specific settings
type AWSConfig struct {
	Region  string `yaml:"region" json:"region" mapstructure:"region"`
	Profile string `yaml:"profile" json:"profile" mapstructure:"profile"`
}

// DeploymentConfig holds deployment settings
type DeploymentConfig struct {
	StackName string          `yaml:"stack_name" json:"stack_name" mapstructure:"stack_name"`
	Mode      PerformanceMode `yaml:"mode" json:"mode" mapstructure:"mode"`
}

// ProxyConfig holds proxy settings
type ProxyConfig struct {
	Port       int    `yaml:"port" json:"port" mapstructure:"port"`
	STUNServer string `yaml:"stun_server" json:"stun_server" mapstructure:"stun_server"`
}


// Merge merges another CLIConfig into this one, with the other taking precedence
func (c *CLIConfig) Merge(other *CLIConfig) {
	if other.AWS.Region != "" {
		c.AWS.Region = other.AWS.Region
	}
	if other.AWS.Profile != "" {
		c.AWS.Profile = other.AWS.Profile
	}
	
	if other.Deployment.StackName != "" {
		c.Deployment.StackName = other.Deployment.StackName
	}
	if other.Deployment.Mode != "" {
		c.Deployment.Mode = other.Deployment.Mode
	}
	
	if other.Proxy.Port != 0 {
		c.Proxy.Port = other.Proxy.Port
	}
	if other.Proxy.STUNServer != "" {
		c.Proxy.STUNServer = other.Proxy.STUNServer
	}
}

// ToLegacyConfig converts CLIConfig to the legacy Config format
// The S3 bucket name should be passed separately since it's auto-detected
func (c *CLIConfig) ToLegacyConfig(s3BucketName string) *Config {
	// Get mode configuration
	modeConfigs := GetModeConfigs()
	modeConfig := modeConfigs[c.Deployment.Mode]
	
	return &Config{
		AWSRegion:             c.AWS.Region,
		S3BucketName:          s3BucketName,
		STUNServer:            c.Proxy.STUNServer,
		SOCKS5Port:            c.Proxy.Port,
		LambdaResponseTimeout: 30 * time.Second, // Keep existing defaults
		NATHolePunchTimeout:   30 * time.Second,
		Rotation: RotationConfig{
			OverlapWindow: modeConfig.OverlapWindow,
			DrainTimeout:  modeConfig.DrainTimeout,
			SessionTTL:    modeConfig.SessionTTL,
		},
		Mode:       c.Deployment.Mode,
		ModeConfig: modeConfig,
	}
}