package config

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)

// DefaultCLIConfig returns a CLIConfig with all default values
func DefaultCLIConfig() *CLIConfig {
	return &CLIConfig{
		AWS: AWSConfig{
			Region:  shared.DefaultAWSRegion,
			Profile: "", // Use default AWS credential chain
		},
		Deployment: DeploymentConfig{
			StackName: generateDefaultStackName(),
			Mode:      ModeNormal,
		},
		Proxy: ProxyConfig{
			Port:       shared.DefaultSOCKS5Port,
			STUNServer: shared.DefaultSTUNServer,
		},
	}
}

// generateDefaultStackName creates a unique stack name with a random suffix
func generateDefaultStackName() string {
	// Generate 4 random bytes for an 8-character hex suffix
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to simple default if random generation fails
		return "lambda-nat-proxy"
	}
	suffix := hex.EncodeToString(bytes)
	return "lambda-nat-proxy-" + suffix
}

// ValidateCLIConfig validates a CLIConfig and returns any errors
func ValidateCLIConfig(cfg *CLIConfig) []error {
	var errors []error
	
	// Validate AWS region
	if cfg.AWS.Region == "" {
		errors = append(errors, &ConfigError{
			Field:   "aws.region",
			Value:   cfg.AWS.Region,
			Message: "AWS region cannot be empty",
		})
	} else {
		// Validate AWS region format (basic check)
		validRegions := []string{
			"us-east-1", "us-east-2", "us-west-1", "us-west-2",
			"eu-central-1", "eu-west-1", "eu-west-2", "eu-west-3",
			"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2",
			"ca-central-1", "sa-east-1", "ap-south-1",
		}
		validRegion := false
		for _, region := range validRegions {
			if cfg.AWS.Region == region {
				validRegion = true
				break
			}
		}
		if !validRegion {
			errors = append(errors, &ConfigError{
				Field:   "aws.region",
				Value:   cfg.AWS.Region,
				Message: "invalid AWS region format",
			})
		}
	}
	
	// Validate deployment mode
	validModes := []PerformanceMode{ModeTest, ModeNormal, ModePerformance}
	validMode := false
	for _, mode := range validModes {
		if cfg.Deployment.Mode == mode {
			validMode = true
			break
		}
	}
	if !validMode {
		errors = append(errors, &ConfigError{
			Field:   "deployment.mode",
			Value:   string(cfg.Deployment.Mode),
			Message: "mode must be one of: test, normal, performance",
		})
	}
	
	// Validate proxy port with additional constraints
	if cfg.Proxy.Port < 1 || cfg.Proxy.Port > 65535 {
		errors = append(errors, &ConfigError{
			Field:   "proxy.port",
			Value:   cfg.Proxy.Port,
			Message: "port must be between 1 and 65535",
		})
	} else if cfg.Proxy.Port < 1024 {
		// Warn about privileged ports
		errors = append(errors, &ConfigError{
			Field:   "proxy.port",
			Value:   cfg.Proxy.Port,
			Message: "ports below 1024 require root privileges",
		})
	}
	
	// Validate STUN server
	if cfg.Proxy.STUNServer == "" {
		errors = append(errors, &ConfigError{
			Field:   "proxy.stun_server",
			Value:   cfg.Proxy.STUNServer,
			Message: "STUN server cannot be empty",
		})
	} else {
		// Validate STUN server format (should be host:port)
		if !strings.Contains(cfg.Proxy.STUNServer, ":") {
			errors = append(errors, &ConfigError{
				Field:   "proxy.stun_server",
				Value:   cfg.Proxy.STUNServer,
				Message: "STUN server must be in format host:port",
			})
		}
	}
	
	// Validate stack name
	if cfg.Deployment.StackName == "" {
		errors = append(errors, &ConfigError{
			Field:   "deployment.stack_name",
			Value:   cfg.Deployment.StackName,
			Message: "stack name cannot be empty",
		})
	} else {
		// Validate stack name constraints per CloudFormation requirements
		if len(cfg.Deployment.StackName) > 128 {
			errors = append(errors, &ConfigError{
				Field:   "deployment.stack_name",
				Value:   cfg.Deployment.StackName,
				Message: "stack name must be 128 characters or less",
			})
		}
		// Check for invalid characters (CloudFormation only allows alphanumeric and hyphens)
		for _, char := range cfg.Deployment.StackName {
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || 
				 (char >= '0' && char <= '9') || char == '-') {
				errors = append(errors, &ConfigError{
					Field:   "deployment.stack_name",
					Value:   cfg.Deployment.StackName,
					Message: "stack name can only contain letters, numbers, and hyphens",
				})
				break
			}
		}
		// Stack name cannot start or end with hyphen
		if strings.HasPrefix(cfg.Deployment.StackName, "-") || strings.HasSuffix(cfg.Deployment.StackName, "-") {
			errors = append(errors, &ConfigError{
				Field:   "deployment.stack_name",
				Value:   cfg.Deployment.StackName,
				Message: "stack name cannot start or end with a hyphen",
			})
		}
	}
	
	// S3 bucket name is auto-detected from CloudFormation stack
	
	return errors
}

// ConfigError represents a configuration validation error
type ConfigError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

// GetDefaultStackName returns the default stack name
func GetDefaultStackName() string {
	return generateDefaultStackName()
}

// GetDefaultBucketName returns the default S3 bucket name based on stack name and account ID
func GetDefaultBucketName(stackName, accountID string) string {
	return stackName + "-coordination-" + accountID
}