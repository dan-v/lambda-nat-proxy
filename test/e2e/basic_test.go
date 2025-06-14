package e2e

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"testing"
	"time"
)

// TestBasicConnectivity tests that the proxy can be deployed and passes basic traffic
func TestBasicConnectivity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 1: Build lambda-nat-proxy
	t.Log("Building lambda-nat-proxy...")
	if err := buildLambdaProxy(); err != nil {
		t.Fatalf("Failed to build lambda-nat-proxy: %v", err)
	}

	// Step 2: Deploy infrastructure
	t.Log("Deploying infrastructure...")
	if err := deployInfrastructure(); err != nil {
		t.Fatalf("Failed to deploy infrastructure: %v", err)
	}

	// Step 3: Start proxy
	t.Log("Starting lambda-nat-proxy...")
	proxyCmd, err := startLambdaProxy(ctx)
	if err != nil {
		t.Fatalf("Failed to start lambda-nat-proxy: %v", err)
	}
	defer func() {
		if proxyCmd.Process != nil {
			proxyCmd.Process.Kill()
		}
	}()

	// Step 4: Wait for proxy to be ready
	t.Log("Waiting for proxy to be ready...")
	if err := waitForSOCKS5(ctx, 8080, 2*time.Minute); err != nil {
		t.Fatalf("Proxy failed to start: %v", err)
	}

	// Step 5: Test basic HTTP traffic through proxy
	t.Log("Testing HTTP traffic through proxy...")
	if err := testHTTPTraffic(); err != nil {
		t.Fatalf("HTTP traffic test failed: %v", err)
	}

	t.Log("✅ Basic connectivity test passed!")
}

// buildLambdaProxy builds the lambda-nat-proxy binary
func buildLambdaProxy() error {
	cmd := exec.Command("make", "build")
	cmd.Dir = "../.."
	return cmd.Run()
}

// deployInfrastructure deploys AWS infrastructure using CLI
func deployInfrastructure() error {
	cmd := exec.Command("./build/lambda-nat-proxy", "deploy")
	cmd.Dir = "../.."
	return cmd.Run()
}

// startLambdaProxy starts the lambda-nat-proxy process
func startLambdaProxy(ctx context.Context) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "./build/lambda-nat-proxy", "run", "--mode", "test")
	cmd.Dir = "../.."
	
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	
	return cmd, nil
}

// waitForSOCKS5 waits for the SOCKS5 proxy to be available
func waitForSOCKS5(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Try to connect to the SOCKS5 proxy
		proxyURL, _ := url.Parse("socks5://localhost:8080")
		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
			Timeout: 5 * time.Second,
		}
		
		// Try a simple HTTP request through the proxy
		resp, err := client.Get("http://httpbin.org/ip")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		
		time.Sleep(5 * time.Second)
	}
	
	return context.DeadlineExceeded
}

// testHTTPTraffic tests that HTTP requests work through the proxy
func testHTTPTraffic() error {
	// Set up HTTP client with SOCKS5 proxy
	proxyURL, _ := url.Parse("socks5://localhost:8080")
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 30 * time.Second,
	}
	
	// Test HTTP request
	resp, err := client.Get("http://httpbin.org/ip")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	// Read response to make sure it works
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	
	if resp.StatusCode != 200 {
		return http.ErrNotSupported
	}
	
	return nil
}