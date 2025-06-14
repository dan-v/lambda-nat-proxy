package shared

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// CreateAWSSession creates a new AWS session with the specified region
func CreateAWSSession(region string) (*session.Session, error) {
	return session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
}

// CreateS3Client creates a new S3 client with the specified region
func CreateS3Client(region string) (*s3.S3, error) {
	sess, err := CreateAWSSession(region)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}
	return s3.New(sess), nil
}

// PutCoordinationData writes coordination data to S3
func PutCoordinationData(s3Client *s3.S3, bucket, sessionID string, data CoordinationData) error {
	coordinationData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal coordination data: %w", err)
	}

	coordinationKey := fmt.Sprintf(CoordinationKeyPattern, sessionID)
	_, err = s3Client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(coordinationKey),
		Body:   strings.NewReader(string(coordinationData)),
	})
	if err != nil {
		return fmt.Errorf("failed to write coordination data to S3: %w", err)
	}

	return nil
}

// GetCoordinationData reads and parses coordination data from S3
func GetCoordinationData(s3Client *s3.S3, bucket, key string) (*CoordinationData, error) {
	obj, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object: %w", err)
	}
	defer obj.Body.Close()

	var coord CoordinationData
	if err := json.NewDecoder(obj.Body).Decode(&coord); err != nil {
		return nil, fmt.Errorf("failed to decode coordination data: %w", err)
	}

	return &coord, nil
}

// PutLambdaResponse writes lambda response data to S3
func PutLambdaResponse(s3Client *s3.S3, bucket, sessionID string, response LambdaResponse) error {
	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal lambda response: %w", err)
	}

	responseKey := fmt.Sprintf(ResponseKeyPattern, sessionID)
	_, err = s3Client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(responseKey),
		Body:   strings.NewReader(string(responseData)),
	})
	if err != nil {
		return fmt.Errorf("failed to write lambda response to S3: %w", err)
	}

	return nil
}

// WaitForS3Object polls for an S3 object until it exists or timeout is reached
func WaitForS3Object(s3Client *s3.S3, bucket, key string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		obj, err := s3Client.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err == nil {
			defer obj.Body.Close()
			// Object exists, read and return content
			content := make([]byte, 0)
			buffer := make([]byte, 1024)
			for {
				n, readErr := obj.Body.Read(buffer)
				if n > 0 {
					content = append(content, buffer[:n]...)
				}
				if readErr != nil {
					break
				}
			}
			return content, nil
		}
		
		// Sleep before next poll
		time.Sleep(DefaultPollingInterval)
	}
	
	return nil, fmt.Errorf("timeout waiting for S3 object %s/%s", bucket, key)
}

// GetLambdaResponse reads and parses lambda response data from S3
func GetLambdaResponse(s3Client *s3.S3, bucket, sessionID string) (*LambdaResponse, error) {
	responseKey := fmt.Sprintf(ResponseKeyPattern, sessionID)
	
	obj, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(responseKey),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read lambda response from S3: %w", err)
	}
	defer obj.Body.Close()

	var response LambdaResponse
	if err := json.NewDecoder(obj.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode lambda response: %w", err)
	}

	return &response, nil
}