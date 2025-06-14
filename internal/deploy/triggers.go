package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/s3"
	
	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

// TriggerDeployerAPI defines the interface for S3 trigger operations
type TriggerDeployerAPI interface {
	ConfigureS3Triggers(ctx context.Context, bucketName, functionArn string) error
	RemoveS3Triggers(ctx context.Context, bucketName, functionArn string) error
	ValidateTriggerConfiguration(ctx context.Context, bucketName, functionArn string) error
	GetBucketNotifications(ctx context.Context, bucketName string) (*s3.NotificationConfiguration, error)
}

// TriggerDeployer handles S3 trigger configuration
type TriggerDeployer struct {
	clients *awsclients.Clients
	cfg     *config.CLIConfig
}

// NewTriggerDeployer creates a new trigger deployer
func NewTriggerDeployer(clients *awsclients.Clients, cfg *config.CLIConfig) *TriggerDeployer {
	return &TriggerDeployer{
		clients: clients,
		cfg:     cfg,
	}
}

// ConfigureS3Triggers configures S3 bucket notifications to trigger Lambda
func (t *TriggerDeployer) ConfigureS3Triggers(ctx context.Context, bucketName, functionArn string) error {
	// Step 1: Add Lambda permission for S3 to invoke the function
	if err := t.addLambdaPermission(ctx, functionArn, bucketName); err != nil {
		return fmt.Errorf("failed to add Lambda permission: %w", err)
	}
	
	// Step 2: Configure S3 bucket notification
	if err := t.configureBucketNotification(ctx, bucketName, functionArn); err != nil {
		return fmt.Errorf("failed to configure bucket notification: %w", err)
	}
	
	return nil
}

// RemoveS3Triggers removes S3 bucket notifications and Lambda permissions
func (t *TriggerDeployer) RemoveS3Triggers(ctx context.Context, bucketName, functionArn string) error {
	// Remove bucket notification first
	if err := t.removeBucketNotification(ctx, bucketName); err != nil {
		log.Printf("Warning: failed to remove bucket notification: %v", err)
	}
	
	// Remove Lambda permission
	if err := t.removeLambdaPermission(ctx, functionArn); err != nil {
		log.Printf("Warning: failed to remove Lambda permission: %v", err)
	}
	
	return nil
}

func (t *TriggerDeployer) addLambdaPermission(ctx context.Context, functionArn, bucketName string) error {
	statementId := fmt.Sprintf("s3-trigger-%s", t.cfg.Deployment.StackName)
	sourceArn := fmt.Sprintf("arn:aws:s3:::%s", bucketName)
	
	input := &lambda.AddPermissionInput{
		FunctionName: aws.String(functionArn),
		StatementId:  aws.String(statementId),
		Action:       aws.String("lambda:InvokeFunction"),
		Principal:    aws.String("s3.amazonaws.com"),
		SourceArn:    aws.String(sourceArn),
	}
	
	_, err := t.clients.Lambda.AddPermissionWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == lambda.ErrCodeResourceConflictException {
				// Permission already exists, which is fine
				log.Printf("Lambda permission already exists")
				return nil
			}
		}
		return err
	}
	
	log.Printf("Added Lambda permission for S3 bucket: %s", bucketName)
	return nil
}

func (t *TriggerDeployer) removeLambdaPermission(ctx context.Context, functionArn string) error {
	statementId := fmt.Sprintf("s3-trigger-%s", t.cfg.Deployment.StackName)
	
	input := &lambda.RemovePermissionInput{
		FunctionName: aws.String(functionArn),
		StatementId:  aws.String(statementId),
	}
	
	_, err := t.clients.Lambda.RemovePermissionWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == lambda.ErrCodeResourceNotFoundException {
				// Permission doesn't exist, which is fine
				return nil
			}
		}
		return err
	}
	
	log.Printf("Removed Lambda permission")
	return nil
}

func (t *TriggerDeployer) configureBucketNotification(ctx context.Context, bucketName, functionArn string) error {
	// Create notification configuration
	notificationConfig := &s3.NotificationConfiguration{
		LambdaFunctionConfigurations: []*s3.LambdaFunctionConfiguration{
			{
				Id:          aws.String("HolePunchTrigger"),
				LambdaFunctionArn: aws.String(functionArn),
				Events: []*string{
					aws.String("s3:ObjectCreated:*"),
				},
				Filter: &s3.NotificationConfigurationFilter{
					Key: &s3.KeyFilter{
						FilterRules: []*s3.FilterRule{
							{
								Name:  aws.String("prefix"),
								Value: aws.String("coordination/"),
							},
						},
					},
				},
			},
		},
	}
	
	input := &s3.PutBucketNotificationConfigurationInput{
		Bucket:                    aws.String(bucketName),
		NotificationConfiguration: notificationConfig,
	}
	
	_, err := t.clients.S3.PutBucketNotificationConfigurationWithContext(ctx, input)
	if err != nil {
		return err
	}
	
	log.Printf("Configured S3 bucket notification for coordination/ prefix")
	return nil
}

func (t *TriggerDeployer) removeBucketNotification(ctx context.Context, bucketName string) error {
	// Set empty notification configuration to remove all notifications
	input := &s3.PutBucketNotificationConfigurationInput{
		Bucket: aws.String(bucketName),
		NotificationConfiguration: &s3.NotificationConfiguration{},
	}
	
	_, err := t.clients.S3.PutBucketNotificationConfigurationWithContext(ctx, input)
	if err != nil {
		return err
	}
	
	log.Printf("Removed S3 bucket notifications")
	return nil
}

// GetBucketNotifications retrieves current bucket notification configuration
func (t *TriggerDeployer) GetBucketNotifications(ctx context.Context, bucketName string) (*s3.NotificationConfiguration, error) {
	input := &s3.GetBucketNotificationConfigurationRequest{
		Bucket: aws.String(bucketName),
	}
	
	result, err := t.clients.S3.GetBucketNotificationConfigurationWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	
	return result, nil
}

// ValidateTriggerConfiguration validates that S3 triggers are properly configured
func (t *TriggerDeployer) ValidateTriggerConfiguration(ctx context.Context, bucketName, functionArn string) error {
	// Get current notification configuration
	notifications, err := t.GetBucketNotifications(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to get bucket notifications: %w", err)
	}
	
	// Check if our Lambda function is configured
	found := false
	for _, lambdaConfig := range notifications.LambdaFunctionConfigurations {
		if lambdaConfig.LambdaFunctionArn != nil && *lambdaConfig.LambdaFunctionArn == functionArn {
			found = true
			break
		}
	}
	
	if !found {
		return fmt.Errorf("Lambda function not found in bucket notification configuration")
	}
	
	// Validate Lambda permission
	if err := t.validateLambdaPermission(ctx, functionArn, bucketName); err != nil {
		return fmt.Errorf("Lambda permission validation failed: %w", err)
	}
	
	return nil
}

func (t *TriggerDeployer) validateLambdaPermission(ctx context.Context, functionArn, bucketName string) error {
	input := &lambda.GetPolicyInput{
		FunctionName: aws.String(functionArn),
	}
	
	result, err := t.clients.Lambda.GetPolicyWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == lambda.ErrCodeResourceNotFoundException {
				return fmt.Errorf("no resource policy found on Lambda function")
			}
		}
		return err
	}
	
	// Parse the policy to check for S3 permissions
	var policy map[string]interface{}
	if err := json.Unmarshal([]byte(*result.Policy), &policy); err != nil {
		return fmt.Errorf("failed to parse Lambda policy: %w", err)
	}
	
	// Check if policy allows S3 to invoke the function
	// This is a simplified check - a more robust implementation would
	// parse the policy structure more thoroughly
	policyStr := *result.Policy
	sourceArn := fmt.Sprintf("arn:aws:s3:::%s", bucketName)
	
	if !strings.Contains(policyStr, "s3.amazonaws.com") || !strings.Contains(policyStr, sourceArn) {
		return fmt.Errorf("Lambda policy does not allow S3 bucket %s to invoke function", bucketName)
	}
	
	return nil
}