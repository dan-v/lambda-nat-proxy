package deploy

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	
	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

// StackDeployerAPI defines the interface for stack deployment operations
type StackDeployerAPI interface {
	DeployStack(ctx context.Context, templateBody string) (*StackOutput, error)
	DeleteStack(ctx context.Context) error
	GetStackOutputs(ctx context.Context) (*StackOutput, error)
}

// StackDeployer handles CloudFormation stack operations
type StackDeployer struct {
	clients *awsclients.Clients
	cfg     *config.CLIConfig
}

// NewStackDeployer creates a new stack deployer
func NewStackDeployer(clients *awsclients.Clients, cfg *config.CLIConfig) *StackDeployer {
	return &StackDeployer{
		clients: clients,
		cfg:     cfg,
	}
}

// DeployStack deploys or updates a CloudFormation stack
func (s *StackDeployer) DeployStack(ctx context.Context, templateBody string) (*StackOutput, error) {
	stackName := s.getFullStackName()
	
	log.Printf("Deploying CloudFormation stack: %s", stackName)
	
	exists, err := s.stackExists(ctx, stackName)
	if err != nil {
		return nil, fmt.Errorf("failed to check if stack exists: %w", err)
	}
	
	parameters := s.buildStackParameters()
	
	if exists {
		return s.updateStack(ctx, stackName, templateBody, parameters)
	}
	
	return s.createStack(ctx, stackName, templateBody, parameters)
}

// DeleteStack deletes a CloudFormation stack
func (s *StackDeployer) DeleteStack(ctx context.Context) error {
	stackName := s.getFullStackName()
	
	log.Printf("Deleting CloudFormation stack: %s", stackName)
	
	input := &cloudformation.DeleteStackInput{
		StackName: aws.String(stackName),
	}
	
	_, err := s.clients.CloudFormation.DeleteStackWithContext(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete stack: %w", err)
	}
	
	// Wait for deletion to complete
	log.Printf("Waiting for stack deletion to complete...")
	err = s.waitForStackOperation(ctx, stackName, cloudformation.StackStatusDeleteComplete, 20*time.Minute)
	if err != nil {
		return fmt.Errorf("stack deletion failed: %w", err)
	}
	
	log.Printf("Stack deleted successfully")
	return nil
}

// GetStackOutputs retrieves outputs from a CloudFormation stack
func (s *StackDeployer) GetStackOutputs(ctx context.Context) (*StackOutput, error) {
	stackName := s.getFullStackName()
	
	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	}
	
	result, err := s.clients.CloudFormation.DescribeStacksWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe stack: %w", err)
	}
	
	if len(result.Stacks) == 0 {
		return nil, fmt.Errorf("stack not found: %s", stackName)
	}
	
	stack := result.Stacks[0]
	return s.extractStackOutputs(stack), nil
}

// StackOutput holds important outputs from the CloudFormation stack
type StackOutput struct {
	StackName                string
	CoordinationBucketName   string
	LambdaExecutionRoleArn   string
	StackStatus              string
	CreationTime             *time.Time
	LastUpdatedTime          *time.Time
}

func (s *StackDeployer) createStack(ctx context.Context, stackName, templateBody string, parameters []*cloudformation.Parameter) (*StackOutput, error) {
	log.Printf("Creating new stack...")
	
	input := &cloudformation.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(templateBody),
		Parameters:   parameters,
		Capabilities: []*string{
			aws.String(cloudformation.CapabilityCapabilityNamedIam),
		},
		Tags: []*cloudformation.Tag{
			{
				Key:   aws.String("Project"),
				Value: aws.String("lambda-nat-proxy"),
			},
			{
				Key:   aws.String("Component"),
				Value: aws.String("cloudformation-stack"),
			},
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("lambda-nat-proxy-cli"),
			},
			{
				Key:   aws.String("Environment"),
				Value: aws.String("production"),
			},
			{
				Key:   aws.String("CostCenter"),
				Value: aws.String("lambda-nat-proxy"),
			},
			{
				Key:   aws.String("Owner"),
				Value: aws.String("lambda-nat-proxy-cli"),
			},
			{
				Key:   aws.String("Mode"),
				Value: aws.String(string(s.cfg.Deployment.Mode)),
			},
		},
	}
	
	result, err := s.clients.CloudFormation.CreateStackWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create stack: %w", err)
	}
	
	log.Printf("Stack creation initiated. Stack ID: %s", *result.StackId)
	
	// Wait for creation to complete
	log.Printf("Waiting for stack creation to complete...")
	err = s.waitForStackOperation(ctx, stackName, cloudformation.StackStatusCreateComplete, 10*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("stack creation failed: %w", err)
	}
	
	log.Printf("Stack created successfully")
	return s.GetStackOutputs(ctx)
}

func (s *StackDeployer) updateStack(ctx context.Context, stackName, templateBody string, parameters []*cloudformation.Parameter) (*StackOutput, error) {
	log.Printf("Updating existing stack...")
	
	input := &cloudformation.UpdateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(templateBody),
		Parameters:   parameters,
		Capabilities: []*string{
			aws.String(cloudformation.CapabilityCapabilityNamedIam),
		},
	}
	
	_, err := s.clients.CloudFormation.UpdateStackWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == "ValidationError" && strings.Contains(awsErr.Message(), "No updates are to be performed") {
				log.Printf("No updates needed for stack")
				return s.GetStackOutputs(ctx)
			}
		}
		return nil, fmt.Errorf("failed to update stack: %w", err)
	}
	
	// Wait for update to complete
	log.Printf("Waiting for stack update to complete...")
	err = s.waitForStackOperation(ctx, stackName, cloudformation.StackStatusUpdateComplete, 10*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("stack update failed: %w", err)
	}
	
	log.Printf("Stack updated successfully")
	return s.GetStackOutputs(ctx)
}

func (s *StackDeployer) stackExists(ctx context.Context, stackName string) (bool, error) {
	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	}
	
	_, err := s.clients.CloudFormation.DescribeStacksWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == "ValidationError" {
				return false, nil
			}
		}
		return false, err
	}
	
	return true, nil
}

func (s *StackDeployer) waitForStackOperation(ctx context.Context, stackName, targetStatus string, timeout time.Duration) error {
	checkFn := func() (bool, error) {
		input := &cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		}
		
		result, err := s.clients.CloudFormation.DescribeStacksWithContext(ctx, input)
		if err != nil {
			// If stack is being deleted and we're waiting for DELETE_COMPLETE, 
			// a ValidationError means deletion succeeded
			if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "ValidationError" {
				if targetStatus == cloudformation.StackStatusDeleteComplete {
					return true, nil
				}
			}
			return false, err
		}
		
		if len(result.Stacks) == 0 {
			return false, fmt.Errorf("stack not found")
		}
		
		stack := result.Stacks[0]
		currentStatus := *stack.StackStatus
		
		log.Printf("Stack status: %s", currentStatus)
		
		// Check for failure states
		if strings.Contains(currentStatus, "FAILED") || 
		   strings.Contains(currentStatus, "ROLLBACK") {
			return false, fmt.Errorf("stack operation failed with status: %s", currentStatus)
		}
		
		return currentStatus == targetStatus, nil
	}
	
	return awsclients.WaitForOperation(ctx, checkFn, timeout)
}

func (s *StackDeployer) buildStackParameters() []*cloudformation.Parameter {
	return []*cloudformation.Parameter{
		{
			ParameterKey:   aws.String("StackName"),
			ParameterValue: aws.String(s.cfg.Deployment.StackName),
		},
	}
}

func (s *StackDeployer) extractStackOutputs(stack *cloudformation.Stack) *StackOutput {
	output := &StackOutput{
		StackName:       *stack.StackName,
		StackStatus:     *stack.StackStatus,
		CreationTime:    stack.CreationTime,
		LastUpdatedTime: stack.LastUpdatedTime,
	}
	
	for _, stackOutput := range stack.Outputs {
		if stackOutput.OutputKey == nil || stackOutput.OutputValue == nil {
			continue
		}
		
		switch *stackOutput.OutputKey {
		case "CoordinationBucketName":
			output.CoordinationBucketName = *stackOutput.OutputValue
		case "LambdaExecutionRoleArn":
			output.LambdaExecutionRoleArn = *stackOutput.OutputValue
		}
	}
	
	return output
}

func (s *StackDeployer) getFullStackName() string {
	return s.cfg.Deployment.StackName
}