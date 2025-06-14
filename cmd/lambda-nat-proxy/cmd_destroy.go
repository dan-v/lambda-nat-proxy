package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/spf13/cobra"
	
	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/deploy"
)

// destroyCmd represents the destroy command
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Remove all AWS resources",
	Long: `Remove all AWS resources created by the deploy command.

This command will:
- Empty and delete the S3 coordination bucket
- Delete the Lambda function
- Delete CloudWatch log groups
- Delete the CloudFormation stack

WARNING: This action is irreversible. All data will be lost.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDestroy(cmd)
	},
}

func runDestroy(cmd *cobra.Command) error {
	ctx := context.Background()
	
	// Load configuration
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadCLIConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	
	// Apply command line flag overrides
	if region, _ := cmd.Flags().GetString("region"); cmd.Flags().Changed("region") {
		cfg.AWS.Region = region
	}
	if stackName, _ := cmd.Flags().GetString("stack-name"); cmd.Flags().Changed("stack-name") {
		cfg.Deployment.StackName = stackName
	}
	
	// Validate configuration
	if errors := config.ValidateCLIConfig(cfg); len(errors) > 0 {
		fmt.Printf("Configuration validation errors:\n")
		for _, err := range errors {
			fmt.Printf("  - %s\n", err.Error())
		}
		return fmt.Errorf("configuration validation failed")
	}
	
	stackName := cfg.Deployment.StackName
	
	// Create AWS clients
	clientFactory, err := awsclients.NewClientFactory(cfg)
	if err != nil {
		return fmt.Errorf("failed to create AWS clients: %w", err)
	}
	
	// Validate AWS credentials
	if err := clientFactory.ValidateCredentials(ctx); err != nil {
		return fmt.Errorf("invalid AWS credentials: %w", err)
	}
	
	clients := clientFactory.GetClients()
	
	// Get stack information first to determine what to clean up
	stackDeployer := deploy.NewStackDeployer(clients, cfg)
	stackOutput, err := stackDeployer.GetStackOutputs(ctx)
	if err != nil {
		log.Printf("Warning: Could not get stack information: %v", err)
		log.Printf("Will attempt to clean up resources by name...")
	}
	
	// Show what will be destroyed
	fmt.Printf("\n🔥 Lambda NAT Proxy Destruction Plan\n")
	fmt.Printf("===================================\n\n")
	fmt.Printf("The following resources will be PERMANENTLY DELETED:\n\n")
	
	if stackOutput != nil {
		fmt.Printf("📦 CloudFormation Stack: %s\n", stackOutput.StackName)
		fmt.Printf("🪣 S3 Bucket: %s\n", stackOutput.CoordinationBucketName)
		fmt.Printf("⚡ Lambda Function: %s-lambda\n", cfg.Deployment.StackName)
		fmt.Printf("📋 CloudWatch Logs: /aws/lambda/%s-lambda\n", cfg.Deployment.StackName)
	} else {
		fmt.Printf("📦 CloudFormation Stack: %s (if exists)\n", stackName)
		fmt.Printf("⚡ Lambda Function: %s-lambda (if exists)\n", cfg.Deployment.StackName)
		fmt.Printf("📋 CloudWatch Logs: /aws/lambda/%s-lambda (if exists)\n", cfg.Deployment.StackName)
	}
	
	fmt.Printf("\n⚠️  WARNING: This action cannot be undone!\n")
	fmt.Printf("💀 All data and configurations will be permanently lost.\n\n")
	
	// Check for --force flag
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		fmt.Printf("Type 'yes' to continue with destruction: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		
		if strings.TrimSpace(strings.ToLower(input)) != "yes" {
			fmt.Println("Destruction cancelled.")
			return nil
		}
	}
	
	fmt.Printf("\n🚀 Starting destruction process...\n\n")
	
	keepLogs, _ := cmd.Flags().GetBool("keep-logs")
	
	// Step 1: Remove S3 triggers and empty bucket
	if stackOutput != nil && stackOutput.CoordinationBucketName != "" {
		if err := cleanupS3Resources(ctx, clients, cfg, stackOutput.CoordinationBucketName); err != nil {
			log.Printf("Warning: S3 cleanup failed: %v", err)
		}
	}
	
	// Step 2: Delete Lambda function
	lambdaDeployer := deploy.NewLambdaDeployer(clients, cfg)
	log.Printf("Step 1/3: Deleting Lambda function...")
	if err := lambdaDeployer.DeleteLambdaFunction(ctx); err != nil {
		log.Printf("Warning: Lambda deletion failed: %v", err)
	} else {
		log.Printf("✅ Lambda function deleted")
	}
	
	// Step 3: Delete CloudWatch logs (unless --keep-logs is specified)
	if !keepLogs {
		functionName := fmt.Sprintf("%s-lambda", cfg.Deployment.StackName)
		log.Printf("Step 2/3: Deleting CloudWatch logs...")
		if err := deleteCloudWatchLogs(ctx, clients, functionName); err != nil {
			log.Printf("Warning: CloudWatch logs deletion failed: %v", err)
		} else {
			log.Printf("✅ CloudWatch logs deleted")
		}
	} else {
		log.Printf("Step 2/3: Skipping CloudWatch logs (--keep-logs specified)")
	}
	
	// Step 4: Delete CloudFormation stack
	log.Printf("Step 3/3: Deleting CloudFormation stack...")
	if err := stackDeployer.DeleteStack(ctx); err != nil {
		log.Printf("Warning: Stack deletion failed: %v", err)
	} else {
		log.Printf("✅ CloudFormation stack deleted")
	}
	
	// Final status
	fmt.Printf("\n🎉 Destruction completed!\n")
	fmt.Printf("All AWS resources have been removed.\n")
	if keepLogs {
		fmt.Printf("\nNote: CloudWatch logs were preserved as requested.\n")
	}
	fmt.Printf("\nYou can run 'lambda-nat-proxy status' to verify all resources are gone.\n")
	
	return nil
}

func cleanupS3Resources(ctx context.Context, clients *awsclients.Clients, cfg *config.CLIConfig, bucketName string) error {
	log.Printf("Cleaning up S3 bucket: %s", bucketName)
	
	// Remove S3 triggers first
	triggerDeployer := deploy.NewTriggerDeployer(clients, cfg)
	functionName := fmt.Sprintf("%s-lambda", cfg.Deployment.StackName)
	functionArn := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", 
		cfg.AWS.Region, 
		clients.AccountID, 
		functionName)
	
	if err := triggerDeployer.RemoveS3Triggers(ctx, bucketName, functionArn); err != nil {
		log.Printf("Warning: Failed to remove S3 triggers: %v", err)
	}
	
	// Empty the bucket
	log.Printf("Emptying S3 bucket...")
	if err := emptyS3Bucket(ctx, clients.S3, bucketName); err != nil {
		return fmt.Errorf("failed to empty S3 bucket: %w", err)
	}
	
	log.Printf("✅ S3 bucket emptied")
	return nil
}

func emptyS3Bucket(ctx context.Context, s3Client awsclients.S3API, bucketName string) error {
	// List all objects
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	}
	
	for {
		result, err := s3Client.ListObjectsV2WithContext(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}
		
		if len(result.Contents) == 0 {
			break
		}
		
		// Delete objects in batch
		var objects []*s3.ObjectIdentifier
		for _, obj := range result.Contents {
			objects = append(objects, &s3.ObjectIdentifier{
				Key: obj.Key,
			})
		}
		
		deleteInput := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &s3.Delete{
				Objects: objects,
			},
		}
		
		_, err = s3Client.DeleteObjectsWithContext(ctx, deleteInput)
		if err != nil {
			return fmt.Errorf("failed to delete objects: %w", err)
		}
		
		log.Printf("Deleted %d objects from bucket", len(objects))
		
		// Check if there are more objects
		if !*result.IsTruncated {
			break
		}
		input.ContinuationToken = result.NextContinuationToken
	}
	
	return nil
}

func deleteCloudWatchLogs(ctx context.Context, clients *awsclients.Clients, functionName string) error {
	logGroupName := fmt.Sprintf("/aws/lambda/%s", functionName)
	
	input := &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	}
	
	_, err := clients.CloudWatchLogs.DeleteLogGroupWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == cloudwatchlogs.ErrCodeResourceNotFoundException {
				log.Printf("CloudWatch log group %s does not exist", logGroupName)
				return nil
			}
		}
		return fmt.Errorf("failed to delete CloudWatch log group: %w", err)
	}
	
	return nil
}

func init() {
	// Add destroy-specific flags
	destroyCmd.Flags().StringP("region", "r", "", "AWS region (overrides config)")
	destroyCmd.Flags().StringP("stack-name", "s", "", "CloudFormation stack name")
	destroyCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
	destroyCmd.Flags().BoolP("keep-logs", "", false, "Keep CloudWatch logs after destroying other resources")
}