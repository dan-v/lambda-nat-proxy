package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)

// executeCliCommand executes the cobra CLI
func executeCliCommand() error {
	return rootCmd.Execute()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "lambda-nat-proxy",
	Short: "A QUIC NAT Traversal SOCKS5 Proxy using AWS Lambda",
	Long: `lambda-nat-proxy is a high-performance SOCKS5 proxy that uses QUIC protocol
and AWS Lambda for NAT traversal. It provides seamless network connectivity
through NAT and firewall restrictions.`,
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Long:  "Print the version information for lambda-nat-proxy",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("lambda-nat-proxy v1.0.0")
	},
}

func init() {
	// Initialize structured logging for CLI
	shared.InitLogger(&shared.LogConfig{
		Level:       shared.LevelInfo,
		Format:      "text", // Human-readable format for CLI
		AddSource:   false,
		ServiceName: "lambda-nat-proxy-cli",
	})
	
	// Add global flags
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path")
	
	// Disable completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	
	// Add commands to root
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
}