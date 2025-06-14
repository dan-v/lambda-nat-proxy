package deploy

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

// LambdaBinaryProvider provides access to Lambda binary data
type LambdaBinaryProvider interface {
	GetLambdaBinary() []byte
}

// LambdaBuilder handles building Lambda deployment packages
type LambdaBuilder struct {
	cfg            *config.CLIConfig
	binaryProvider LambdaBinaryProvider
}

// NewLambdaBuilder creates a new Lambda builder
func NewLambdaBuilder(cfg *config.CLIConfig) *LambdaBuilder {
	return &LambdaBuilder{
		cfg: cfg,
	}
}

// NewLambdaBuilderWithProvider creates a new Lambda builder with a binary provider
func NewLambdaBuilderWithProvider(cfg *config.CLIConfig, provider LambdaBinaryProvider) *LambdaBuilder {
	return &LambdaBuilder{
		cfg:            cfg,
		binaryProvider: provider,
	}
}

// BuildResult contains information about a Lambda build
type BuildResult struct {
	ZipPath     string
	Size        int64
	BuildTime   time.Duration
	CacheHit    bool
}

// BuildLambdaPackage builds the Lambda function deployment package using embedded binary
func (b *LambdaBuilder) BuildLambdaPackage(buildDir, lambdaDir string) (*BuildResult, error) {
	startTime := time.Now()
	
	// Create build directory if it doesn't exist
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create build directory: %w", err)
	}
	
	zipPath := filepath.Join(buildDir, "lambda-function.zip")
	
	// Create deployment package from embedded binary
	if err := b.createDeploymentPackageFromEmbedded(zipPath); err != nil {
		return nil, fmt.Errorf("failed to create deployment package: %w", err)
	}
	
	info, err := os.Stat(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat zip: %w", err)
	}
	
	return &BuildResult{
		ZipPath:   zipPath,
		Size:      info.Size(),
		BuildTime: time.Since(startTime),
		CacheHit:  false,
	}, nil
}

// BuildLambdaPackageFromSource builds the Lambda function deployment package from source (legacy)
func (b *LambdaBuilder) BuildLambdaPackageFromSource(buildDir, lambdaDir string) (*BuildResult, error) {
	startTime := time.Now()
	
	// Create build directory if it doesn't exist
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create build directory: %w", err)
	}
	
	binaryPath := filepath.Join(buildDir, "bootstrap")
	zipPath := filepath.Join(buildDir, "lambda-function.zip")
	
	// Check if we can use cached build
	if b.canUseCachedBuild(binaryPath, zipPath, lambdaDir) {
		info, err := os.Stat(zipPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat cached zip: %w", err)
		}
		
		return &BuildResult{
			ZipPath:   zipPath,
			Size:      info.Size(),
			BuildTime: time.Since(startTime),
			CacheHit:  true,
		}, nil
	}
	
	// Build the Lambda binary
	if err := b.buildLambdaBinary(lambdaDir, binaryPath); err != nil {
		return nil, fmt.Errorf("failed to build Lambda binary: %w", err)
	}
	
	// Create the deployment package
	if err := b.createDeploymentPackage(binaryPath, zipPath); err != nil {
		return nil, fmt.Errorf("failed to create deployment package: %w", err)
	}
	
	info, err := os.Stat(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat zip file: %w", err)
	}
	
	return &BuildResult{
		ZipPath:   zipPath,
		Size:      info.Size(),
		BuildTime: time.Since(startTime),
		CacheHit:  false,
	}, nil
}

// buildLambdaBinary builds the Lambda binary for linux/amd64
func (b *LambdaBuilder) buildLambdaBinary(lambdaDir, outputPath string) error {
	cmd := exec.Command("go", "build", "-o", outputPath, ".")
	cmd.Dir = lambdaDir
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build failed: %w\nOutput: %s", err, string(output))
	}
	
	// Ensure the binary is executable
	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}
	
	return nil
}

// createDeploymentPackage creates a zip file with the Lambda binary
func (b *LambdaBuilder) createDeploymentPackage(binaryPath, zipPath string) error {
	// Remove existing zip file
	if err := os.Remove(zipPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing zip: %w", err)
	}
	
	// Create new zip file
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()
	
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()
	
	// Add the bootstrap binary to the zip
	return b.addFileToZip(zipWriter, binaryPath, "bootstrap")
}

// createDeploymentPackageFromEmbedded creates a zip file with the embedded Lambda binary
func (b *LambdaBuilder) createDeploymentPackageFromEmbedded(zipPath string) error {
	// Remove existing zip file
	if err := os.Remove(zipPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing zip: %w", err)
	}
	
	// Create new zip file
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()
	
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()
	
	// Add the embedded bootstrap binary to the zip
	return b.addEmbeddedBinaryToZip(zipWriter, "bootstrap")
}

// addEmbeddedBinaryToZip adds the embedded Lambda binary to a zip archive
func (b *LambdaBuilder) addEmbeddedBinaryToZip(zipWriter *zip.Writer, nameInZip string) error {
	if b.binaryProvider == nil {
		return fmt.Errorf("no binary provider available")
	}
	
	// Get the embedded binary
	binaryData := b.binaryProvider.GetLambdaBinary()
	if len(binaryData) == 0 {
		return fmt.Errorf("embedded Lambda binary is empty - was it built before CLI compilation?")
	}
	
	// Create zip file header
	header := &zip.FileHeader{
		Name:   nameInZip,
		Method: zip.Deflate,
	}
	
	// Set executable permissions and modification time
	header.SetMode(0755)
	header.Modified = time.Now()
	
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create zip entry: %w", err)
	}
	
	// Write the embedded binary data
	_, err = io.Copy(writer, bytes.NewReader(binaryData))
	if err != nil {
		return fmt.Errorf("failed to copy embedded binary to zip: %w", err)
	}
	
	return nil
}

// addFileToZip adds a file to a zip archive
func (b *LambdaBuilder) addFileToZip(zipWriter *zip.Writer, filePath, nameInZip string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()
	
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("failed to create zip header: %w", err)
	}
	
	header.Name = nameInZip
	header.Method = zip.Deflate
	
	// Set executable permissions
	header.SetMode(0755)
	
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create zip entry: %w", err)
	}
	
	_, err = io.Copy(writer, file)
	if err != nil {
		return fmt.Errorf("failed to copy file to zip: %w", err)
	}
	
	return nil
}

// canUseCachedBuild checks if we can use a cached build based on modification times
func (b *LambdaBuilder) canUseCachedBuild(binaryPath, zipPath, lambdaDir string) bool {
	// Check if zip file exists
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return false
	}
	
	// Check if binary exists
	binaryInfo, err := os.Stat(binaryPath)
	if err != nil {
		return false
	}
	
	// Find the newest source file in lambda directory
	newestSourceTime, err := b.findNewestSourceFile(lambdaDir)
	if err != nil {
		return false
	}
	
	// Cache is valid if zip is newer than sources and binary
	return zipInfo.ModTime().After(newestSourceTime) && zipInfo.ModTime().After(binaryInfo.ModTime())
}

// findNewestSourceFile finds the modification time of the newest source file
func (b *LambdaBuilder) findNewestSourceFile(dir string) (time.Time, error) {
	var newestTime time.Time
	
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Check Go source files and go.mod
		if filepath.Ext(path) == ".go" || filepath.Base(path) == "go.mod" || filepath.Base(path) == "go.sum" {
			if info.ModTime().After(newestTime) {
				newestTime = info.ModTime()
			}
		}
		
		return nil
	})
	
	return newestTime, err
}

// GetPackageInfo returns information about an existing package
func (b *LambdaBuilder) GetPackageInfo(zipPath string) (*BuildResult, error) {
	info, err := os.Stat(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat package: %w", err)
	}
	
	return &BuildResult{
		ZipPath: zipPath,
		Size:    info.Size(),
	}, nil
}