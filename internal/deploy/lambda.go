package deploy

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/lambda"
	
	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

// LambdaDeployerAPI defines the interface for Lambda deployment operations
type LambdaDeployerAPI interface {
	DeployLambdaFunction(ctx context.Context, zipPath, roleArn string) (*LambdaDeployResult, error)
	DeleteLambdaFunction(ctx context.Context) error
	GetFunctionInfo(ctx context.Context) (*LambdaDeployResult, error)
}

// LambdaDeployer handles Lambda function deployment
type LambdaDeployer struct {
	clients *awsclients.Clients
	cfg     *config.CLIConfig
}

// NewLambdaDeployer creates a new Lambda deployer
func NewLambdaDeployer(clients *awsclients.Clients, cfg *config.CLIConfig) *LambdaDeployer {
	return &LambdaDeployer{
		clients: clients,
		cfg:     cfg,
	}
}

// LambdaDeployResult contains information about a Lambda deployment
type LambdaDeployResult struct {
	FunctionName    string
	FunctionArn     string
	Runtime         string
	MemorySize      int64
	Timeout         int64
	LastModified    string
	CodeSize        int64
	State           string
}

// DeployLambdaFunction deploys or updates a Lambda function
func (d *LambdaDeployer) DeployLambdaFunction(ctx context.Context, zipPath, roleArn string) (*LambdaDeployResult, error) {
	functionName := d.getFunctionName()
	
	log.Printf("Deploying Lambda function: %s", functionName)
	
	// Read the deployment package
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read deployment package: %w", err)
	}
	
	exists, err := d.functionExists(ctx, functionName)
	if err != nil {
		return nil, fmt.Errorf("failed to check if function exists: %w", err)
	}
	
	if exists {
		return d.updateFunction(ctx, functionName, zipData)
	}
	
	return d.createFunction(ctx, functionName, zipData, roleArn)
}

// DeleteLambdaFunction deletes a Lambda function
func (d *LambdaDeployer) DeleteLambdaFunction(ctx context.Context) error {
	functionName := d.getFunctionName()
	
	log.Printf("Deleting Lambda function: %s", functionName)
	
	input := &lambda.DeleteFunctionInput{
		FunctionName: aws.String(functionName),
	}
	
	_, err := d.clients.Lambda.DeleteFunctionWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == lambda.ErrCodeResourceNotFoundException {
				log.Printf("Function %s does not exist", functionName)
				return nil
			}
		}
		return fmt.Errorf("failed to delete function: %w", err)
	}
	
	log.Printf("Lambda function deleted successfully")
	return nil
}

// GetFunctionInfo retrieves information about a Lambda function
func (d *LambdaDeployer) GetFunctionInfo(ctx context.Context) (*LambdaDeployResult, error) {
	functionName := d.getFunctionName()
	
	input := &lambda.GetFunctionInput{
		FunctionName: aws.String(functionName),
	}
	
	result, err := d.clients.Lambda.GetFunctionWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == lambda.ErrCodeResourceNotFoundException {
				return nil, fmt.Errorf("function not found: %s", functionName)
			}
		}
		return nil, fmt.Errorf("failed to get function: %w", err)
	}
	
	return d.extractFunctionInfo(result.Configuration), nil
}

func (d *LambdaDeployer) createFunction(ctx context.Context, functionName string, zipData []byte, roleArn string) (*LambdaDeployResult, error) {
	log.Printf("Creating new Lambda function...")
	
	modeConfig := config.GetModeConfigs()[d.cfg.Deployment.Mode]
	
	input := &lambda.CreateFunctionInput{
		FunctionName: aws.String(functionName),
		Runtime:      aws.String(lambda.RuntimeProvidedAl2),
		Role:         aws.String(roleArn),
		Handler:      aws.String("bootstrap"),
		Code: &lambda.FunctionCode{
			ZipFile: zipData,
		},
		Timeout:     aws.Int64(int64(modeConfig.LambdaTimeout)),
		MemorySize:  aws.Int64(int64(modeConfig.LambdaMemory)),
		Description: aws.String(fmt.Sprintf("QUIC NAT Proxy Lambda (%s mode)", d.cfg.Deployment.Mode)),
		Environment: &lambda.Environment{
			Variables: map[string]*string{
				"MODE": aws.String(string(d.cfg.Deployment.Mode)),
			},
		},
		Tags: map[string]*string{
			"Project":     aws.String("lambda-nat-proxy"),
			"Component":   aws.String("lambda-function"),
			"Mode":        aws.String(string(d.cfg.Deployment.Mode)),
			"ManagedBy":   aws.String("lambda-nat-proxy-cli"),
			"Environment": aws.String("production"),
			"CostCenter":  aws.String("lambda-nat-proxy"),
			"Owner":       aws.String("lambda-nat-proxy-cli"),
			"Runtime":     aws.String(lambda.RuntimeProvidedAl2),
		},
	}
	
	result, err := d.clients.Lambda.CreateFunctionWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create function: %w", err)
	}
	
	log.Printf("Function created. ARN: %s", *result.FunctionArn)
	
	// Wait for function to be active
	if err := d.waitForFunctionActive(ctx, functionName); err != nil {
		return nil, fmt.Errorf("function creation failed: %w", err)
	}
	
	log.Printf("Lambda function created successfully")
	return d.extractFunctionInfo(result), nil
}

func (d *LambdaDeployer) updateFunction(ctx context.Context, functionName string, zipData []byte) (*LambdaDeployResult, error) {
	log.Printf("Updating existing Lambda function...")
	
	// Update function code
	codeInput := &lambda.UpdateFunctionCodeInput{
		FunctionName: aws.String(functionName),
		ZipFile:      zipData,
	}
	
	_, err := d.clients.Lambda.UpdateFunctionCodeWithContext(ctx, codeInput)
	if err != nil {
		return nil, fmt.Errorf("failed to update function code: %w", err)
	}
	
	// Wait for update to complete
	if err := d.waitForFunctionUpdated(ctx, functionName); err != nil {
		return nil, fmt.Errorf("function code update failed: %w", err)
	}
	
	// Update function configuration if needed
	modeConfig := config.GetModeConfigs()[d.cfg.Deployment.Mode]
	
	configInput := &lambda.UpdateFunctionConfigurationInput{
		FunctionName: aws.String(functionName),
		Timeout:      aws.Int64(int64(modeConfig.LambdaTimeout)),
		MemorySize:   aws.Int64(int64(modeConfig.LambdaMemory)),
		Environment: &lambda.Environment{
			Variables: map[string]*string{
				"MODE": aws.String(string(d.cfg.Deployment.Mode)),
			},
		},
	}
	
	configResult, err := d.clients.Lambda.UpdateFunctionConfigurationWithContext(ctx, configInput)
	if err != nil {
		return nil, fmt.Errorf("failed to update function configuration: %w", err)
	}
	
	// Wait for configuration update to complete
	if err := d.waitForFunctionUpdated(ctx, functionName); err != nil {
		return nil, fmt.Errorf("function configuration update failed: %w", err)
	}
	
	log.Printf("Lambda function updated successfully")
	
	// Return the configuration result since it's more recent
	return d.extractFunctionInfo(configResult), nil
}

func (d *LambdaDeployer) functionExists(ctx context.Context, functionName string) (bool, error) {
	input := &lambda.GetFunctionInput{
		FunctionName: aws.String(functionName),
	}
	
	_, err := d.clients.Lambda.GetFunctionWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == lambda.ErrCodeResourceNotFoundException {
				return false, nil
			}
		}
		return false, err
	}
	
	return true, nil
}

func (d *LambdaDeployer) waitForFunctionActive(ctx context.Context, functionName string) error {
	log.Printf("Waiting for function to become active...")
	
	checkFn := func() (bool, error) {
		input := &lambda.GetFunctionInput{
			FunctionName: aws.String(functionName),
		}
		
		result, err := d.clients.Lambda.GetFunctionWithContext(ctx, input)
		if err != nil {
			return false, err
		}
		
		state := *result.Configuration.State
		log.Printf("Function state: %s", state)
		
		if state == lambda.StateFailed {
			return false, fmt.Errorf("function is in failed state")
		}
		
		return state == lambda.StateActive, nil
	}
	
	return awsclients.WaitForOperation(ctx, checkFn, 5*time.Minute)
}

func (d *LambdaDeployer) waitForFunctionUpdated(ctx context.Context, functionName string) error {
	log.Printf("Waiting for function update to complete...")
	
	checkFn := func() (bool, error) {
		input := &lambda.GetFunctionInput{
			FunctionName: aws.String(functionName),
		}
		
		result, err := d.clients.Lambda.GetFunctionWithContext(ctx, input)
		if err != nil {
			return false, err
		}
		
		lastUpdateStatus := *result.Configuration.LastUpdateStatus
		log.Printf("Function update status: %s", lastUpdateStatus)
		
		if lastUpdateStatus == lambda.LastUpdateStatusFailed {
			return false, fmt.Errorf("function update failed")
		}
		
		return lastUpdateStatus == lambda.LastUpdateStatusSuccessful, nil
	}
	
	return awsclients.WaitForOperation(ctx, checkFn, 5*time.Minute)
}

func (d *LambdaDeployer) extractFunctionInfo(config *lambda.FunctionConfiguration) *LambdaDeployResult {
	result := &LambdaDeployResult{
		FunctionName: *config.FunctionName,
		FunctionArn:  *config.FunctionArn,
		Runtime:      *config.Runtime,
		MemorySize:   *config.MemorySize,
		Timeout:      *config.Timeout,
		LastModified: *config.LastModified,
		CodeSize:     *config.CodeSize,
		State:        *config.State,
	}
	
	return result
}

func (d *LambdaDeployer) getFunctionName() string {
	return fmt.Sprintf("%s-lambda", d.cfg.Deployment.StackName)
}