package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/metrics"
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)

// Coordinator handles coordination with Lambda via S3
type Coordinator interface {
	WriteCoordination(ctx context.Context, sessionID, publicIP string, port int) error
	WaitForLambdaResponse(ctx context.Context, sessionID string, timeout time.Duration) (*shared.LambdaResponse, error)
}

// DefaultCoordinator implements Coordinator
type DefaultCoordinator struct {
	s3Client   awsclients.S3API
	bucketName string
}

// New creates a new S3 coordinator
func New(s3Client awsclients.S3API, bucketName string) Coordinator {
	return &DefaultCoordinator{
		s3Client:   s3Client,
		bucketName: bucketName,
	}
}

// WriteCoordination writes coordination data to S3 to trigger Lambda
func (c *DefaultCoordinator) WriteCoordination(ctx context.Context, sessionID, publicIP string, port int) error {
	coord := shared.CoordinationData{
		SessionID:        sessionID,
		LaptopPublicIP:   publicIP,
		LaptopPublicPort: port,
		Timestamp:        time.Now().Unix(),
	}

	coordData, err := json.Marshal(coord)
	if err != nil {
		return fmt.Errorf("failed to marshal coordination data: %w", err)
	}

	s3Key := fmt.Sprintf(shared.CoordinationKeyPattern, sessionID)

	start := time.Now()
	_, err = c.s3Client.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(s3Key),
		Body:   bytes.NewReader(coordData),
	})
	
	// Record S3 operation metrics
	metrics.RecordS3Operation()
	metrics.RecordAWSAPILatency(time.Since(start))

	if err != nil {
		metrics.RecordS3Error()
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case s3.ErrCodeNoSuchBucket:
				return fmt.Errorf("S3 bucket '%s' does not exist. Please run 'lambda-nat-proxy deploy' to create infrastructure", c.bucketName)
			case "AccessDenied":
				return fmt.Errorf("access denied to S3 bucket '%s'. Please check AWS credentials have S3 permissions:\n\n"+
					"Required permissions:\n"+
					"- s3:PutObject\n"+
					"- s3:GetObject\n"+
					"- s3:DeleteObject", c.bucketName)
			case "InvalidBucketName":
				return fmt.Errorf("invalid S3 bucket name '%s'. Bucket names must be DNS-compliant", c.bucketName)
			default:
				return fmt.Errorf("S3 operation failed (%s): %v\nBucket: %s\nKey: coordination/%s", 
					awsErr.Code(), awsErr.Message(), c.bucketName, sessionID)
			}
		}
		return fmt.Errorf("failed to write to S3: %w", err)
	}

	return nil
}

// WaitForLambdaResponse polls S3 for Lambda response
func (c *DefaultCoordinator) WaitForLambdaResponse(ctx context.Context, sessionID string, timeout time.Duration) (*shared.LambdaResponse, error) {
	deadline := time.Now().Add(timeout)
	responseKey := fmt.Sprintf(shared.ResponseKeyPattern, sessionID)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		start := time.Now()
		obj, err := c.s3Client.GetObjectWithContext(ctx, &s3.GetObjectInput{
			Bucket: aws.String(c.bucketName),
			Key:    aws.String(responseKey),
		})
		
		// Record S3 operation metrics
		metrics.RecordS3Operation()
		metrics.RecordAWSAPILatency(time.Since(start))

		if err == nil {
			defer obj.Body.Close()

			var response shared.LambdaResponse
			if err := json.NewDecoder(obj.Body).Decode(&response); err == nil {
				metrics.RecordLambdaInvocation()
				return &response, nil
			}
		} else {
			// Only record S3 error for actual errors, not "not found" which is expected
			if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() != s3.ErrCodeNoSuchKey {
				metrics.RecordS3Error()
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(shared.ResponsePollInterval):
		}
	}

	return nil, fmt.Errorf("timeout waiting for Lambda response")
}