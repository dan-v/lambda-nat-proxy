package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	
	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/deploy"
)

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy infrastructure and Lambda function",
	Long: `Deploy the AWS infrastructure and Lambda function needed for the proxy.

This command will:
- Deploy CloudFormation stack with S3 bucket and IAM roles
- Build and deploy the Lambda function
- Configure S3 trigger for Lambda invocation
- Set appropriate memory and timeout based on performance mode

The deployment process typically takes 2-5 minutes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDeploy(cmd)
	},
}

func runDeploy(cmd *cobra.Command) error {
	ctx := context.Background()
	
	// Load configuration
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadCLIConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	
	// Apply command line flag overrides
	if mode, _ := cmd.Flags().GetString("mode"); cmd.Flags().Changed("mode") {
		cfg.Deployment.Mode = config.PerformanceMode(mode)
	}
	if region, _ := cmd.Flags().GetString("region"); cmd.Flags().Changed("region") {
		cfg.AWS.Region = region
	}
	if stackName, _ := cmd.Flags().GetString("stack-name"); cmd.Flags().Changed("stack-name") {
		cfg.Deployment.StackName = stackName
	}
	
	// Validate configuration
	if errors := config.ValidateCLIConfig(cfg); len(errors) > 0 {
		fmt.Printf("❌ Configuration validation failed:\n\n")
		for _, err := range errors {
			errMsg := err.Error()
			fmt.Printf("  • %s\n", errMsg)
			// Add specific guidance based on common configuration issues
			if strings.Contains(errMsg, "region") {
				fmt.Printf("    💡 Set region with: --region us-west-2 or in config file\n")
			} else if strings.Contains(errMsg, "mode") {
				fmt.Printf("    💡 Valid modes: test, normal, performance\n")
			} else if strings.Contains(errMsg, "stack") {
				fmt.Printf("    💡 Stack names must be 1-128 chars, letters/numbers/hyphens only\n")
			}
		}
		fmt.Printf("\n💡 Generate a sample config file with: lambda-nat-proxy config init\n")
		return fmt.Errorf("please fix the configuration issues above")
	}
	
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		return runDeployDryRun(cfg)
	}
	
	log.Printf("Starting deployment in %s mode...", cfg.Deployment.Mode)
	log.Printf("AWS Region: %s", cfg.AWS.Region)
	log.Printf("Stack: %s", cfg.Deployment.StackName)
	
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
	
	// Step 1: Deploy CloudFormation stack
	log.Printf("Step 1/3: Deploying CloudFormation stack...")
	stackDeployer := deploy.NewStackDeployer(clients, cfg)
	
	template, err := deploy.GetCloudFormationTemplate(cfg, "")
	if err != nil {
		return fmt.Errorf("failed to get CloudFormation template: %w", err)
	}
	
	stackOutput, err := stackDeployer.DeployStack(ctx, template)
	if err != nil {
		return fmt.Errorf("failed to deploy stack: %w", err)
	}
	
	log.Printf("✅ Stack deployed successfully")
	log.Printf("   S3 Bucket: %s", stackOutput.CoordinationBucketName)
	
	// Step 2: Build and deploy Lambda function
	log.Printf("Step 2/3: Building and deploying Lambda function...")
	
	// Use embedded Lambda binary
	provider := &EmbeddedLambdaProvider{}
	builder := deploy.NewLambdaBuilderWithProvider(cfg, provider)
	buildResult, err := builder.BuildLambdaPackage("build", "lambda")
	if err != nil {
		return fmt.Errorf("failed to build Lambda package: %w", err)
	}
	
	if buildResult.CacheHit {
		log.Printf("✅ Using cached Lambda package (%d bytes)", buildResult.Size)
	} else {
		log.Printf("✅ Lambda package built in %v (%d bytes)", buildResult.BuildTime, buildResult.Size)
	}
	
	lambdaDeployer := deploy.NewLambdaDeployer(clients, cfg)
	lambdaResult, err := lambdaDeployer.DeployLambdaFunction(ctx, buildResult.ZipPath, stackOutput.LambdaExecutionRoleArn)
	if err != nil {
		return fmt.Errorf("failed to deploy Lambda function: %w", err)
	}
	
	log.Printf("✅ Lambda function deployed successfully")
	log.Printf("   Function: %s", lambdaResult.FunctionName)
	log.Printf("   Memory: %d MB", lambdaResult.MemorySize)
	log.Printf("   Timeout: %d seconds", lambdaResult.Timeout)
	
	// Step 3: Configure S3 triggers
	log.Printf("Step 3/3: Configuring S3 triggers...")
	
	triggerDeployer := deploy.NewTriggerDeployer(clients, cfg)
	if err := triggerDeployer.ConfigureS3Triggers(ctx, stackOutput.CoordinationBucketName, lambdaResult.FunctionArn); err != nil {
		return fmt.Errorf("failed to configure S3 triggers: %w", err)
	}
	
	log.Printf("✅ S3 triggers configured successfully")
	
	// Display deployment summary
	fmt.Println("\n🎉 Deployment completed successfully!")
	fmt.Printf("Stack Name: %s\n", stackOutput.StackName)
	fmt.Printf("Region: %s\n", cfg.AWS.Region)
	fmt.Printf("S3 Bucket: %s\n", stackOutput.CoordinationBucketName)
	fmt.Printf("Lambda Function: %s\n", lambdaResult.FunctionName)
	fmt.Printf("Performance Mode: %s\n", cfg.Deployment.Mode)
	fmt.Println("\nYou can now run the proxy with:")
	fmt.Printf("  lambda-nat-proxy run\n")
	
	return nil
}

func runDeployDryRun(cfg *config.CLIConfig) error {
	fmt.Println("🔍 Dry run - showing what would be deployed:")
	fmt.Printf("Stack Name: %s\n", cfg.Deployment.StackName)
	fmt.Printf("AWS Region: %s\n", cfg.AWS.Region)
	fmt.Printf("Performance Mode: %s\n", cfg.Deployment.Mode)
	
	modeConfig := config.GetModeConfigs()[cfg.Deployment.Mode]
	fmt.Printf("Lambda Memory: %d MB\n", modeConfig.LambdaMemory)
	fmt.Printf("Lambda Timeout: %d seconds\n", modeConfig.LambdaTimeout)
	fmt.Printf("Session TTL: %v\n", modeConfig.SessionTTL)
	
	fmt.Println("\nDeployment steps that would be performed:")
	fmt.Println("1. Deploy CloudFormation stack with S3 bucket and IAM roles")
	fmt.Println("2. Build Lambda deployment package")
	fmt.Println("3. Deploy Lambda function with performance mode settings")
	fmt.Println("4. Configure S3 bucket notifications to trigger Lambda")
	
	fmt.Println("\nTo perform actual deployment, run without --dry-run flag")
	
	return nil
}

func init() {
	// Add deploy-specific flags
	deployCmd.Flags().StringP("mode", "m", "normal", "Performance mode (test, normal, performance)")
	deployCmd.Flags().StringP("region", "r", "", "AWS region (overrides config)")
	deployCmd.Flags().StringP("stack-name", "s", "", "CloudFormation stack name")
	deployCmd.Flags().BoolP("dry-run", "", false, "Show what would be deployed without actually deploying")
}