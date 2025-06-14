package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	awss3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/spf13/cobra"
	
	awsclients "github.com/dan-v/lambda-nat-punch-proxy/internal/aws"
	"github.com/dan-v/lambda-nat-punch-proxy/internal"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/dashboard"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/deploy"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/manager"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/metrics"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/nat"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/quic"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/s3"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/socks5"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/stun"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the SOCKS5 proxy server",
	Long: `Start the SOCKS5 proxy server with QUIC NAT traversal.

This command starts the proxy server which will:
- Establish QUIC connections through AWS Lambda
- Perform NAT hole punching for connectivity
- Start a SOCKS5 proxy server on the specified port
- Launch the dashboard web interface (auto-opens in browser)
- Handle automatic session rotation and failover

The proxy will run until stopped with Ctrl+C.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProxy(cmd)
	},
}

func runProxy(cmd *cobra.Command) error {
	// Load configuration
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadCLIConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	
	// Apply command line flag overrides
	if port, _ := cmd.Flags().GetInt("port"); cmd.Flags().Changed("port") {
		cfg.Proxy.Port = port
	}
	if mode, _ := cmd.Flags().GetString("mode"); cmd.Flags().Changed("mode") {
		cfg.Deployment.Mode = config.PerformanceMode(mode)
	}
	
	// Validate configuration
	if errors := config.ValidateCLIConfig(cfg); len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Configuration validation errors:\n")
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", err.Error())
		}
		return fmt.Errorf("configuration validation failed")
	}
	
	// Auto-detect S3 bucket from CloudFormation stack
	bucketName, err := autoDetectS3Bucket(cfg)
	if err != nil {
		return fmt.Errorf("unable to find S3 bucket. Please deploy infrastructure first:\n\n  lambda-nat-proxy deploy\n\nError details: %v", err)
	}
	
	// Convert to legacy config format
	legacyConfig := cfg.ToLegacyConfig(bucketName)
	
	// Set up debug logging if requested
	if debug, _ := cmd.Flags().GetBool("debug"); debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Printf("Debug mode enabled")
		log.Printf("Using configuration: AWS region=%s, S3 bucket=%s, SOCKS5 port=%d, mode=%s", 
			cfg.AWS.Region, bucketName, cfg.Proxy.Port, cfg.Deployment.Mode)
	}
	
	log.Printf("Using S3 bucket: %s", legacyConfig.S3BucketName)
	log.Printf("Using AWS region: %s", legacyConfig.AWSRegion)
	
	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(legacyConfig.AWSRegion),
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %w", err)
	}
	
	// Initialize components
	stunClient := stun.New()
	s3Coord := s3.New(awss3.New(sess), legacyConfig.S3BucketName)
	natTraversal := nat.New()
	socks5Proxy := socks5.New()
	quicServer := quic.New()
	
	// Create launcher for session management
	launcher := internal.NewLauncher(legacyConfig, stunClient, s3Coord, natTraversal, quicServer)
	
	// Create connection manager
	cm := manager.New(legacyConfig, launcher)
	
	// Create context with interrupt handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	
	// Start connection manager in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- cm.Start(ctx)
	}()
	
	// Wait for the first session to be established
	waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer waitCancel()
	
	log.Printf("Establishing initial session...")
	if _, err := cm.WaitForSession(waitCtx); err != nil {
		cancel()
		if err == context.DeadlineExceeded {
			return fmt.Errorf("timeout establishing initial session after 30 seconds.\n\n"+
				"🔧 Troubleshooting steps:\n"+
				"1. Check AWS Lambda function status: lambda-nat-proxy status\n"+
				"2. Verify S3 bucket permissions and triggers\n"+
				"3. Check CloudWatch logs: lambda-nat-proxy status --logs\n"+
				"4. Ensure firewall allows outbound UDP traffic\n"+
				"5. Try a different performance mode: --mode test")
		}
		return fmt.Errorf("failed to establish initial session: %w\n\n"+
			"💡 Run 'lambda-nat-proxy status' to check infrastructure health", err)
	}
	log.Printf("Initial session established successfully")
	
	// Start comprehensive metrics server if debug mode or metrics flag
	debug, _ := cmd.Flags().GetBool("debug")
	enableMetrics, _ := cmd.Flags().GetBool("metrics")
	enableDashboard, _ := cmd.Flags().GetBool("dashboard")
	noBrowser, _ := cmd.Flags().GetBool("no-browser")
	
	if debug || enableMetrics {
		go func() {
			log.Println("🔍 Starting comprehensive metrics server on :6060")
			log.Println("📊 Metrics available at:")
			log.Println("   - http://localhost:6060/metrics (Prometheus format)")
			log.Println("   - http://localhost:6060/debug/vars (JSON format)")
			
			if err := metrics.StartMetricsServer(":6060"); err != nil && err != http.ErrServerClosed {
				log.Printf("❌ Metrics server error: %v", err)
			}
		}()
	}
	
	// Start dashboard server if requested
	var dashboardServer *dashboard.DashboardServer
	if enableDashboard {
		// Start connection tracking metrics collection
		dashboard.StartMetricsCollection()
		
		dashboardServer = dashboard.NewDashboardServer(cm)
		go func() {
			log.Println("🎨 Starting dashboard server on :8081")
			log.Println("🌐 Dashboard available at: http://localhost:8081")
			
			httpServer := &http.Server{
				Addr:         ":8081",
				Handler:      dashboardServer,
				ReadTimeout:  15 * time.Second,
				WriteTimeout: 15 * time.Second,
			}
			
			// Auto-open dashboard in browser after a short delay (unless disabled)
			if !noBrowser {
				go func() {
					time.Sleep(2 * time.Second) // Wait for server to start
					openBrowser("http://localhost:8081")
				}()
			}
			
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("❌ Dashboard server error: %v", err)
			}
		}()
	}
	
	// Start SOCKS5 proxy in background with context
	go func() {
		log.Printf("Starting SOCKS5 proxy on port %d", legacyConfig.SOCKS5Port)
		if err := socks5Proxy.StartWithConnManagerAndContext(ctx, legacyConfig.SOCKS5Port, cm); err != nil {
			if ctx.Err() == nil { // Only log error if not due to context cancellation
				log.Printf("SOCKS5 proxy error: %v", err)
			}
			cancel()
		}
	}()
	
	log.Printf("Proxy is ready! Use SOCKS5 proxy at localhost:%d", legacyConfig.SOCKS5Port)
	
	// Wait for connection manager to finish or interrupt
	err = <-errCh
	
	// Handle graceful shutdown on interrupt
	if err != nil && ctx.Err() == context.Canceled {
		log.Printf("Shutting down...")
		
		// Create a timeout context for graceful shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		
		// Stop dashboard and metrics collection immediately
		if enableDashboard && dashboardServer != nil {
			log.Printf("Shutting down dashboard server...")
			dashboardServer.Shutdown()
			dashboard.StopMetricsCollection()
		}
		
		// Give minimal time for connections to close
		select {
		case <-shutdownCtx.Done():
			log.Printf("Shutdown timeout reached")
		case <-time.After(500 * time.Millisecond):
			log.Printf("Proxy stopped gracefully")
		}
		return nil
	}
	
	return err
}

// autoDetectS3Bucket attempts to detect the S3 bucket from CloudFormation stack
func autoDetectS3Bucket(cfg *config.CLIConfig) (string, error) {
	// Create AWS clients
	clientFactory, err := awsclients.NewClientFactory(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to create AWS clients: %w", err)
	}
	
	// Try to get stack outputs
	clients := clientFactory.GetClients()
	stackDeployer := deploy.NewStackDeployer(clients, cfg)
	
	stackOutput, err := stackDeployer.GetStackOutputs(context.Background())
	if err != nil {
		// Provide more helpful error message with multiple options
		return "", fmt.Errorf("CloudFormation stack '%s' not found in region %s.\n\n"+
			"📦 To deploy infrastructure:\n"+
			"   lambda-nat-proxy deploy\n\n"+
			"🔍 To check existing deployments:\n"+
			"   lambda-nat-proxy status\n\n"+
			"⚙️  To use a different stack name:\n"+
			"   lambda-nat-proxy run --stack-name your-stack-name", 
			cfg.Deployment.StackName, cfg.AWS.Region)
	}
	
	if stackOutput.CoordinationBucketName == "" {
		return "", fmt.Errorf("S3 bucket not found in CloudFormation stack outputs")
	}
	
	return stackOutput.CoordinationBucketName, nil
}

func init() {
	// Add run-specific flags
	runCmd.Flags().IntP("port", "p", 8080, "SOCKS5 proxy port")
	runCmd.Flags().BoolP("debug", "d", false, "Enable debug logging")
	runCmd.Flags().Bool("metrics", false, "Enable metrics server on port 6060")
	runCmd.Flags().Bool("dashboard", true, "Enable dashboard web UI on port 8081")
	runCmd.Flags().Bool("no-browser", false, "Disable auto-opening dashboard in browser")
	runCmd.Flags().StringP("mode", "m", "normal", "Performance mode (test, normal, performance)")
}

// openBrowser opens the specified URL in the user's default browser
func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	
	args = append(args, url)
	
	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("🌐 Unable to auto-open browser: %v", err)
		log.Printf("🌐 Please manually open: %s", url)
	} else {
		log.Printf("🚀 Dashboard opening in your browser...")
	}
}