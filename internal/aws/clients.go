package aws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

// CloudFormationAPI defines the interface for CloudFormation operations
type CloudFormationAPI interface {
	CreateStackWithContext(ctx context.Context, input *cloudformation.CreateStackInput, opts ...request.Option) (*cloudformation.CreateStackOutput, error)
	UpdateStackWithContext(ctx context.Context, input *cloudformation.UpdateStackInput, opts ...request.Option) (*cloudformation.UpdateStackOutput, error)
	DeleteStackWithContext(ctx context.Context, input *cloudformation.DeleteStackInput, opts ...request.Option) (*cloudformation.DeleteStackOutput, error)
	DescribeStacksWithContext(ctx context.Context, input *cloudformation.DescribeStacksInput, opts ...request.Option) (*cloudformation.DescribeStacksOutput, error)
}

// CloudWatchLogsAPI defines the interface for CloudWatch Logs operations
type CloudWatchLogsAPI interface {
	GetLogEventsWithContext(ctx context.Context, input *cloudwatchlogs.GetLogEventsInput, opts ...request.Option) (*cloudwatchlogs.GetLogEventsOutput, error)
	DescribeLogGroupsWithContext(ctx context.Context, input *cloudwatchlogs.DescribeLogGroupsInput, opts ...request.Option) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
	DescribeLogStreamsWithContext(ctx context.Context, input *cloudwatchlogs.DescribeLogStreamsInput, opts ...request.Option) (*cloudwatchlogs.DescribeLogStreamsOutput, error)
	DeleteLogGroupWithContext(ctx context.Context, input *cloudwatchlogs.DeleteLogGroupInput, opts ...request.Option) (*cloudwatchlogs.DeleteLogGroupOutput, error)
}

// LambdaAPI defines the interface for Lambda operations
type LambdaAPI interface {
	CreateFunctionWithContext(ctx context.Context, input *lambda.CreateFunctionInput, opts ...request.Option) (*lambda.FunctionConfiguration, error)
	UpdateFunctionCodeWithContext(ctx context.Context, input *lambda.UpdateFunctionCodeInput, opts ...request.Option) (*lambda.FunctionConfiguration, error)
	UpdateFunctionConfigurationWithContext(ctx context.Context, input *lambda.UpdateFunctionConfigurationInput, opts ...request.Option) (*lambda.FunctionConfiguration, error)
	DeleteFunctionWithContext(ctx context.Context, input *lambda.DeleteFunctionInput, opts ...request.Option) (*lambda.DeleteFunctionOutput, error)
	GetFunctionWithContext(ctx context.Context, input *lambda.GetFunctionInput, opts ...request.Option) (*lambda.GetFunctionOutput, error)
	AddPermissionWithContext(ctx context.Context, input *lambda.AddPermissionInput, opts ...request.Option) (*lambda.AddPermissionOutput, error)
	RemovePermissionWithContext(ctx context.Context, input *lambda.RemovePermissionInput, opts ...request.Option) (*lambda.RemovePermissionOutput, error)
	GetPolicyWithContext(ctx context.Context, input *lambda.GetPolicyInput, opts ...request.Option) (*lambda.GetPolicyOutput, error)
}

// S3API defines the interface for S3 operations
type S3API interface {
	PutBucketNotificationConfigurationWithContext(ctx context.Context, input *s3.PutBucketNotificationConfigurationInput, opts ...request.Option) (*s3.PutBucketNotificationConfigurationOutput, error)
	GetBucketNotificationConfigurationWithContext(ctx context.Context, input *s3.GetBucketNotificationConfigurationRequest, opts ...request.Option) (*s3.NotificationConfiguration, error)
	ListObjectsV2WithContext(ctx context.Context, input *s3.ListObjectsV2Input, opts ...request.Option) (*s3.ListObjectsV2Output, error)
	DeleteObjectWithContext(ctx context.Context, input *s3.DeleteObjectInput, opts ...request.Option) (*s3.DeleteObjectOutput, error)
	DeleteObjectsWithContext(ctx context.Context, input *s3.DeleteObjectsInput, opts ...request.Option) (*s3.DeleteObjectsOutput, error)
	PutObjectWithContext(ctx context.Context, input *s3.PutObjectInput, opts ...request.Option) (*s3.PutObjectOutput, error)
	GetObjectWithContext(ctx context.Context, input *s3.GetObjectInput, opts ...request.Option) (*s3.GetObjectOutput, error)
}

// STSAPI defines the interface for STS operations
type STSAPI interface {
	GetCallerIdentityWithContext(ctx context.Context, input *sts.GetCallerIdentityInput, opts ...request.Option) (*sts.GetCallerIdentityOutput, error)
}

// ClientFactory creates and manages AWS service clients
type ClientFactory struct {
	session   *session.Session
	accountID string
	mu        sync.RWMutex
}

// Clients holds all AWS service clients
type Clients struct {
	CloudFormation CloudFormationAPI
	CloudWatchLogs CloudWatchLogsAPI
	Lambda         LambdaAPI
	S3             S3API
	STS            STSAPI
	AccountID      string
}

// NewClientFactory creates a new AWS client factory
func NewClientFactory(cfg *config.CLIConfig) (*ClientFactory, error) {
	awsConfig := &aws.Config{
		Region: aws.String(cfg.AWS.Region),
	}
	
	// Add retry configuration
	awsConfig.Retryer = client.DefaultRetryer{
		NumMaxRetries:    5,
		MinRetryDelay:    100 * time.Millisecond,
		MinThrottleDelay: 500 * time.Millisecond,
		MaxRetryDelay:    5 * time.Second,
		MaxThrottleDelay: 30 * time.Second,
	}
	
	// Set profile if specified
	sessionOpts := session.Options{
		Config:            *awsConfig,
		SharedConfigState: session.SharedConfigEnable,
	}
	
	if cfg.AWS.Profile != "" {
		sessionOpts.Profile = cfg.AWS.Profile
	}
	
	sess, err := session.NewSessionWithOptions(sessionOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}
	
	factory := &ClientFactory{
		session: sess,
	}
	
	return factory, nil
}

// GetClients returns all AWS service clients
func (f *ClientFactory) GetClients() *Clients {
	// Get account ID
	accountID, _ := f.GetAccountID(context.Background())
	
	return &Clients{
		CloudFormation: cloudformation.New(f.session),
		CloudWatchLogs: cloudwatchlogs.New(f.session),
		Lambda:         lambda.New(f.session),
		S3:             s3.New(f.session),
		STS:            sts.New(f.session),
		AccountID:      accountID,
	}
}

// GetAccountID returns the AWS account ID, caching the result
func (f *ClientFactory) GetAccountID(ctx context.Context) (string, error) {
	f.mu.RLock()
	if f.accountID != "" {
		accountID := f.accountID
		f.mu.RUnlock()
		return accountID, nil
	}
	f.mu.RUnlock()
	
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Double-check after acquiring write lock
	if f.accountID != "" {
		return f.accountID, nil
	}
	
	stsClient := sts.New(f.session)
	input := &sts.GetCallerIdentityInput{}
	
	result, err := stsClient.GetCallerIdentityWithContext(ctx, input)
	if err != nil {
		// Check for common AWS credential errors
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case "NoCredentialsErr":
				return "", fmt.Errorf("AWS credentials not found. Please run 'aws configure' or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables")
			case "TokenRefreshRequired":
				return "", fmt.Errorf("AWS credentials have expired. Please refresh your credentials or run 'aws sso login' if using SSO")
			case "UnauthorizedOperation":
				return "", fmt.Errorf("AWS credentials lack necessary permissions. Ensure your AWS user/role has CloudFormation, Lambda, and S3 permissions")
			case "InvalidUserID.NotFound":
				return "", fmt.Errorf("AWS credentials are invalid. Please check your AWS access key and secret key")
			default:
				return "", fmt.Errorf("AWS credential validation failed (%s): %v\n\n🔧 Troubleshooting:\n- Verify AWS credentials: aws sts get-caller-identity\n- Check region setting: %s", awsErr.Code(), awsErr.Message(), *f.session.Config.Region)
			}
		}
		return "", fmt.Errorf("failed to validate AWS credentials: %w\n\n💡 Please check your AWS configuration", err)
	}
	
	if result.Account == nil {
		return "", fmt.Errorf("account ID not found in caller identity")
	}
	
	f.accountID = *result.Account
	return f.accountID, nil
}

// ValidateCredentials checks if AWS credentials are valid
func (f *ClientFactory) ValidateCredentials(ctx context.Context) error {
	_, err := f.GetAccountID(ctx)
	return err
}

// GetRegion returns the configured AWS region
func (f *ClientFactory) GetRegion() string {
	return *f.session.Config.Region
}

// WaitForOperation waits for an AWS operation to complete using exponential backoff
func WaitForOperation(ctx context.Context, checkFn func() (bool, error), maxWait time.Duration) error {
	backoff := 2 * time.Second
	maxBackoff := 30 * time.Second
	deadline := time.Now().Add(maxWait)
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		if time.Now().After(deadline) {
			return fmt.Errorf("operation timeout after %v", maxWait)
		}
		
		done, err := checkFn()
		if err != nil {
			return fmt.Errorf("operation check failed: %w", err)
		}
		
		if done {
			return nil
		}
		
		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// AddRetryHandlers adds custom retry handlers to requests
func AddRetryHandlers(req *request.Request) {
	req.Handlers.Retry.PushBack(func(r *request.Request) {
		if r.RetryCount > 0 {
			// Add jitter to prevent thundering herd
			delay := time.Duration(r.RetryCount) * 100 * time.Millisecond
			jitter := time.Duration(r.RetryCount*50) * time.Millisecond
			time.Sleep(delay + jitter)
		}
	})
}