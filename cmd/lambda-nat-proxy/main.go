package main

import (
	"log"
	"strings"
)

func main() {
	// Always use CLI mode
	if err := executeCliCommand(); err != nil {
		// Provide more context-specific error messages
		errMsg := err.Error()
		if strings.Contains(errMsg, "AWS credentials") || strings.Contains(errMsg, "credentials") {
			log.Fatalf("❌ AWS credentials error: %v\n\n🔧 Troubleshooting:\n- Run 'aws configure' to set up credentials\n- Set AWS_PROFILE environment variable\n- Ensure your AWS credentials have the necessary permissions", err)
		} else if strings.Contains(errMsg, "configuration") {
			log.Fatalf("❌ Configuration error: %v\n\n💡 Tip: Run 'lambda-nat-proxy config init' to create a sample configuration file", err)
		} else if strings.Contains(errMsg, "CloudFormation") || strings.Contains(errMsg, "stack") {
			log.Fatalf("❌ Infrastructure error: %v\n\n💡 Try: Run 'lambda-nat-proxy deploy' to set up infrastructure", err)
		} else if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "network") {
			log.Fatalf("❌ Network error: %v\n\n🔧 Check your internet connection and firewall settings", err)
		} else {
			log.Fatalf("❌ Command failed: %v\n\n💡 For help, run: lambda-nat-proxy --help", err)
		}
	}
}

