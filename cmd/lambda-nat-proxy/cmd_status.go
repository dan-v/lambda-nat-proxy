package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	
	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/deploy"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show deployment status",
	Long: `Show the status of deployed AWS resources.

This command displays:
- CloudFormation stack status and outputs
- Lambda function details (memory, timeout, last modified)
- S3 bucket contents and recent activity
- Recent CloudWatch logs

Use this command to verify deployment status and troubleshoot issues.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(cmd)
	},
}

// StatusInfo holds deployment status information
type StatusInfo struct {
	Stack   *StackStatus   `json:"stack,omitempty" yaml:"stack,omitempty"`
	Lambda  *LambdaStatus  `json:"lambda,omitempty" yaml:"lambda,omitempty"`
	S3      *S3Status      `json:"s3,omitempty" yaml:"s3,omitempty"`
	Logs    []LogEntry     `json:"logs,omitempty" yaml:"logs,omitempty"`
	Summary *StatusSummary `json:"summary" yaml:"summary"`
}

type StackStatus struct {
	Name         string     `json:"name" yaml:"name"`
	Status       string     `json:"status" yaml:"status"`
	CreatedAt    *time.Time `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	BucketName   string     `json:"bucket_name,omitempty" yaml:"bucket_name,omitempty"`
	RoleArn      string     `json:"role_arn,omitempty" yaml:"role_arn,omitempty"`
}

type LambdaStatus struct {
	Name         string `json:"name" yaml:"name"`
	State        string `json:"state" yaml:"state"`
	Runtime      string `json:"runtime" yaml:"runtime"`
	MemorySize   int64  `json:"memory_size" yaml:"memory_size"`
	Timeout      int64  `json:"timeout" yaml:"timeout"`
	CodeSize     int64  `json:"code_size" yaml:"code_size"`
	LastModified string `json:"last_modified" yaml:"last_modified"`
}

type S3Status struct {
	BucketName      string `json:"bucket_name" yaml:"bucket_name"`
	ObjectCount     int    `json:"object_count" yaml:"object_count"`
	TotalSize       int64  `json:"total_size_bytes" yaml:"total_size_bytes"`
	LastActivity    string `json:"last_activity,omitempty" yaml:"last_activity,omitempty"`
	NotificationsOK bool   `json:"notifications_configured" yaml:"notifications_configured"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp" yaml:"timestamp"`
	Message   string `json:"message" yaml:"message"`
	Level     string `json:"level,omitempty" yaml:"level,omitempty"`
}

type StatusSummary struct {
	Overall     string `json:"overall" yaml:"overall"`
	StackOK     bool   `json:"stack_ok" yaml:"stack_ok"`
	LambdaOK    bool   `json:"lambda_ok" yaml:"lambda_ok"`
	S3OK        bool   `json:"s3_ok" yaml:"s3_ok"`
	TriggersOK  bool   `json:"triggers_ok" yaml:"triggers_ok"`
	LastUpdated string `json:"last_updated" yaml:"last_updated"`
}

func runStatus(cmd *cobra.Command) error {
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
	
	// Gather status information
	statusInfo := &StatusInfo{
		Summary: &StatusSummary{
			LastUpdated: time.Now().Format("2006-01-02 15:04:05 MST"),
		},
	}
	
	// Get stack status
	stackDeployer := deploy.NewStackDeployer(clients, cfg)
	if stackOutput, err := stackDeployer.GetStackOutputs(ctx); err == nil {
		statusInfo.Stack = &StackStatus{
			Name:       stackOutput.StackName,
			Status:     stackOutput.StackStatus,
			CreatedAt:  stackOutput.CreationTime,
			UpdatedAt:  stackOutput.LastUpdatedTime,
			BucketName: stackOutput.CoordinationBucketName,
			RoleArn:    stackOutput.LambdaExecutionRoleArn,
		}
		statusInfo.Summary.StackOK = stackOutput.StackStatus == "CREATE_COMPLETE" || stackOutput.StackStatus == "UPDATE_COMPLETE"
	} else {
		statusInfo.Summary.StackOK = false
	}
	
	// Get Lambda status
	lambdaDeployer := deploy.NewLambdaDeployer(clients, cfg)
	if lambdaInfo, err := lambdaDeployer.GetFunctionInfo(ctx); err == nil {
		statusInfo.Lambda = &LambdaStatus{
			Name:         lambdaInfo.FunctionName,
			State:        lambdaInfo.State,
			Runtime:      lambdaInfo.Runtime,
			MemorySize:   lambdaInfo.MemorySize,
			Timeout:      lambdaInfo.Timeout,
			CodeSize:     lambdaInfo.CodeSize,
			LastModified: lambdaInfo.LastModified,
		}
		statusInfo.Summary.LambdaOK = lambdaInfo.State == "Active"
	} else {
		statusInfo.Summary.LambdaOK = false
	}
	
	// Get S3 status if stack exists
	if statusInfo.Stack != nil && statusInfo.Stack.BucketName != "" {
		// Get S3 status
		if s3Status, err := getS3Status(ctx, clients, cfg, statusInfo.Stack.BucketName); err == nil {
			statusInfo.S3 = s3Status
			statusInfo.Summary.S3OK = true
		} else {
			statusInfo.Summary.S3OK = false
		}
		
		// Check S3 triggers if Lambda exists
		if statusInfo.Lambda != nil {
			// Check S3 trigger configuration
			triggerDeployer := deploy.NewTriggerDeployer(clients, cfg)
			functionArn := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", 
				cfg.AWS.Region, 
				clients.AccountID, 
				statusInfo.Lambda.Name)
			if err := triggerDeployer.ValidateTriggerConfiguration(ctx, statusInfo.Stack.BucketName, functionArn); err == nil {
				statusInfo.Summary.TriggersOK = true
			} else {
				statusInfo.Summary.TriggersOK = false
			}
		}
	}
	
	// Get recent logs if requested
	showLogs, _ := cmd.Flags().GetBool("logs")
	if showLogs && statusInfo.Lambda != nil {
		// Fetch recent logs
		if logs, err := getRecentLogs(ctx, clients, statusInfo.Lambda.Name); err == nil {
			statusInfo.Logs = logs
		} else {
			// Failed to fetch logs - continue silently
		}
	}
	
	// Determine overall status
	if statusInfo.Summary.StackOK && statusInfo.Summary.LambdaOK && statusInfo.Summary.S3OK {
		if statusInfo.Summary.TriggersOK {
			statusInfo.Summary.Overall = "HEALTHY"
		} else {
			statusInfo.Summary.Overall = "DEGRADED"
		}
	} else {
		statusInfo.Summary.Overall = "UNHEALTHY"
	}
	
	// Output status in requested format
	format, _ := cmd.Flags().GetString("format")
	return outputStatus(statusInfo, format)
}

func getS3Status(ctx context.Context, clients *awsclients.Clients, cfg *config.CLIConfig, bucketName string) (*S3Status, error) {
	status := &S3Status{
		BucketName: bucketName,
	}
	
	// List objects to get count and size
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	}
	
	result, err := clients.S3.ListObjectsV2WithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 objects: %w", err)
	}
	
	status.ObjectCount = len(result.Contents)
	var totalSize int64
	var lastModified *time.Time
	
	for _, obj := range result.Contents {
		totalSize += *obj.Size
		if lastModified == nil || obj.LastModified.After(*lastModified) {
			lastModified = obj.LastModified
		}
	}
	
	status.TotalSize = totalSize
	if lastModified != nil {
		status.LastActivity = lastModified.Format("2006-01-02 15:04:05")
	}
	
	// Check bucket notifications
	triggerDeployer := deploy.NewTriggerDeployer(clients, cfg)
	notificationConfig, err := triggerDeployer.GetBucketNotifications(ctx, bucketName)
	if err == nil {
		status.NotificationsOK = len(notificationConfig.LambdaFunctionConfigurations) > 0
	}
	
	return status, nil
}

func getRecentLogs(ctx context.Context, clients *awsclients.Clients, functionName string) ([]LogEntry, error) {
	logGroupName := fmt.Sprintf("/aws/lambda/%s", functionName)
	
	// Get log streams
	input := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroupName),
		OrderBy:      aws.String("LastEventTime"),
		Descending:   aws.Bool(true),
		Limit:        aws.Int64(5),
	}
	
	streams, err := clients.CloudWatchLogs.DescribeLogStreamsWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get log streams: %w", err)
	}
	
	var entries []LogEntry
	
	// Get events from the most recent stream
	if len(streams.LogStreams) > 0 {
		eventsInput := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(logGroupName),
			LogStreamName: streams.LogStreams[0].LogStreamName,
			StartFromHead: aws.Bool(false),
			Limit:         aws.Int64(20),
		}
		
		events, err := clients.CloudWatchLogs.GetLogEventsWithContext(ctx, eventsInput)
		if err != nil {
			return nil, fmt.Errorf("failed to get log events: %w", err)
		}
		
		for _, event := range events.Events {
			if event.Message != nil && event.Timestamp != nil {
				timestamp := time.Unix(*event.Timestamp/1000, 0).Format("2006-01-02 15:04:05")
				message := strings.TrimSpace(*event.Message)
				
				// Try to extract log level
				level := ""
				if strings.Contains(message, "ERROR") {
					level = "ERROR"
				} else if strings.Contains(message, "WARN") {
					level = "WARN"
				} else if strings.Contains(message, "INFO") {
					level = "INFO"
				} else if strings.Contains(message, "DEBUG") {
					level = "DEBUG"
				}
				
				entries = append(entries, LogEntry{
					Timestamp: timestamp,
					Message:   message,
					Level:     level,
				})
			}
		}
	}
	
	return entries, nil
}

func outputStatus(status *StatusInfo, format string) error {
	switch strings.ToLower(format) {
	case "json":
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		
	case "yaml":
		data, err := yaml.Marshal(status)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		fmt.Print(string(data))
		
	case "table":
		return outputStatusTable(status)
		
	default:
		return fmt.Errorf("unsupported format: %s (use table, json, or yaml)", format)
	}
	
	return nil
}

func outputStatusTable(status *StatusInfo) error {
	fmt.Printf("\n🚀 Lambda NAT Proxy Status\n")
	fmt.Printf("========================\n\n")
	
	// Overall status
	statusEmoji := "❌"
	if status.Summary.Overall == "HEALTHY" {
		statusEmoji = "✅"
	} else if status.Summary.Overall == "DEGRADED" {
		statusEmoji = "⚠️"
	}
	
	fmt.Printf("Overall Status: %s %s\n", statusEmoji, status.Summary.Overall)
	fmt.Printf("Last Updated:   %s\n\n", status.Summary.LastUpdated)
	
	// CloudFormation Stack
	fmt.Printf("📦 CloudFormation Stack\n")
	fmt.Printf("----------------------\n")
	if status.Stack != nil {
		statusIcon := "✅"
		if !status.Summary.StackOK {
			statusIcon = "❌"
		}
		fmt.Printf("Status:      %s %s\n", statusIcon, status.Stack.Status)
		fmt.Printf("Name:        %s\n", status.Stack.Name)
		if status.Stack.CreatedAt != nil {
			fmt.Printf("Created:     %s\n", status.Stack.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		if status.Stack.UpdatedAt != nil {
			fmt.Printf("Updated:     %s\n", status.Stack.UpdatedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("Bucket:      %s\n", status.Stack.BucketName)
	} else {
		fmt.Printf("Status:      ❌ NOT FOUND\n")
	}
	fmt.Println()
	
	// Lambda Function
	fmt.Printf("⚡ Lambda Function\n")
	fmt.Printf("-----------------\n")
	if status.Lambda != nil {
		statusIcon := "✅"
		if !status.Summary.LambdaOK {
			statusIcon = "❌"
		}
		fmt.Printf("Status:      %s %s\n", statusIcon, status.Lambda.State)
		fmt.Printf("Name:        %s\n", status.Lambda.Name)
		fmt.Printf("Runtime:     %s\n", status.Lambda.Runtime)
		fmt.Printf("Memory:      %d MB\n", status.Lambda.MemorySize)
		fmt.Printf("Timeout:     %d seconds\n", status.Lambda.Timeout)
		fmt.Printf("Code Size:   %d bytes\n", status.Lambda.CodeSize)
		fmt.Printf("Modified:    %s\n", status.Lambda.LastModified)
	} else {
		fmt.Printf("Status:      ❌ NOT FOUND\n")
	}
	fmt.Println()
	
	// S3 Bucket
	fmt.Printf("🪣 S3 Bucket\n")
	fmt.Printf("------------\n")
	if status.S3 != nil {
		statusIcon := "✅"
		if !status.Summary.S3OK {
			statusIcon = "❌"
		}
		fmt.Printf("Status:       %s ACCESSIBLE\n", statusIcon)
		fmt.Printf("Name:         %s\n", status.S3.BucketName)
		fmt.Printf("Objects:      %d\n", status.S3.ObjectCount)
		fmt.Printf("Total Size:   %d bytes\n", status.S3.TotalSize)
		if status.S3.LastActivity != "" {
			fmt.Printf("Last Activity:%s\n", status.S3.LastActivity)
		}
		
		notificationIcon := "✅"
		if !status.S3.NotificationsOK {
			notificationIcon = "❌"
		}
		fmt.Printf("Notifications:%s Configured\n", notificationIcon)
	} else {
		fmt.Printf("Status:       ❌ NOT ACCESSIBLE\n")
	}
	fmt.Println()
	
	// S3 Triggers
	fmt.Printf("🔗 S3 Triggers\n")
	fmt.Printf("--------------\n")
	triggerIcon := "✅"
	if !status.Summary.TriggersOK {
		triggerIcon = "❌"
	}
	if status.Lambda != nil && status.S3 != nil {
		fmt.Printf("Status:      %s CONFIGURED\n", triggerIcon)
	} else {
		fmt.Printf("Status:      ❌ NOT AVAILABLE (missing dependencies)\n")
	}
	fmt.Println()
	
	// Recent Logs
	if len(status.Logs) > 0 {
		fmt.Printf("📋 Recent Logs\n")
		fmt.Printf("--------------\n")
		for i, entry := range status.Logs {
			if i >= 10 { // Limit to 10 entries in table view
				break
			}
			levelIcon := "ℹ️"
			if entry.Level == "ERROR" {
				levelIcon = "❌"
			} else if entry.Level == "WARN" {
				levelIcon = "⚠️"
			}
			fmt.Printf("%s [%s] %s\n", levelIcon, entry.Timestamp, entry.Message)
		}
		fmt.Println()
	}
	
	// Summary
	fmt.Printf("💡 Quick Status\n")
	fmt.Printf("---------------\n")
	fmt.Printf("Stack:    %s\n", boolToIcon(status.Summary.StackOK))
	fmt.Printf("Lambda:   %s\n", boolToIcon(status.Summary.LambdaOK))
	fmt.Printf("S3:       %s\n", boolToIcon(status.Summary.S3OK))
	fmt.Printf("Triggers: %s\n", boolToIcon(status.Summary.TriggersOK))
	
	// Show deployment guidance if nothing is deployed
	if status.Summary.Overall == "UNHEALTHY" && !status.Summary.StackOK {
		fmt.Printf("\n💡 Getting Started\n")
		fmt.Printf("------------------\n")
		fmt.Printf("No infrastructure found. To get started:\n\n")
		fmt.Printf("1. Deploy the infrastructure:\n")
		fmt.Printf("   lambda-nat-proxy deploy\n\n")
		fmt.Printf("2. Start the proxy:\n")
		fmt.Printf("   lambda-nat-proxy run\n")
	}
	
	return nil
}

func boolToIcon(b bool) string {
	if b {
		return "✅ OK"
	}
	return "❌ FAIL"
}

func init() {
	// Add status-specific flags
	statusCmd.Flags().StringP("region", "r", "", "AWS region (overrides config)")
	statusCmd.Flags().StringP("stack-name", "s", "", "CloudFormation stack name")
	statusCmd.Flags().StringP("format", "", "table", "Output format (table, json, yaml)")
	statusCmd.Flags().BoolP("logs", "l", false, "Show recent Lambda logs")
}