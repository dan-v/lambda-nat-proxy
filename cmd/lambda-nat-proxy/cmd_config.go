package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	
	"github.com/dan-v/lambda-nat-punch-proxy/internal/config"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
	Long: `Manage lambda-nat-proxy configuration files.

Configuration is loaded from multiple sources in order of precedence:
1. Command line flags
2. Environment variables
3. Configuration file
4. Default values

The configuration file is searched in:
- Current directory (lambda-nat-proxy.yaml)
- ~/.config/lambda-nat-proxy/lambda-nat-proxy.yaml (XDG config home)
- /etc/lambda-nat-proxy/lambda-nat-proxy.yaml (system-wide)`,
}

// configInitCmd represents the config init command
var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration file",
	Long: `Create a new configuration file with default values.

This command creates a lambda-nat-proxy.yaml file in the user's config directory
(~/.config/lambda-nat-proxy/) with all available configuration options and their
default values. You can then edit this file to customize the proxy settings.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigInit(cmd)
	},
}

// configShowCmd represents the config show command
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long: `Display the current configuration values.

This command shows the merged configuration from all sources
(defaults, config file, environment variables, and command line flags)
and indicates where each value comes from.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigShow(cmd)
	},
}

func init() {
	// Add subcommands to config
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	
	// Add config init specific flags
	configInitCmd.Flags().StringP("output", "o", "", "Output file path (defaults to XDG config directory)")
	configInitCmd.Flags().BoolP("force", "f", false, "Overwrite existing config file")
	
	// Add config show specific flags
	configShowCmd.Flags().StringP("format", "", "yaml", "Output format (yaml, json, table)")
}

// runConfigInit implements the config init command
func runConfigInit(cmd *cobra.Command) error {
	outputPath, _ := cmd.Flags().GetString("output")
	force, _ := cmd.Flags().GetBool("force")
	
	// Use default config path if not specified
	if outputPath == "" {
		outputPath = config.GetDefaultConfigPath()
	}
	
	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil && !force {
		fmt.Printf("Configuration file already exists at: %s\n\n", outputPath)
		fmt.Println("What would you like to do?")
		fmt.Println("1. View current config (recommended)")
		fmt.Println("2. Overwrite with fresh defaults")
		fmt.Println("3. Cancel")
		fmt.Print("\nChoose an option [1/2/3]: ")
		
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		
		response = strings.TrimSpace(response)
		switch response {
		case "1", "":
			// Show current config
			fmt.Println("\nCurrent configuration:")
			fmt.Println("─────────────────────")
			
			// Load and display config directly
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.LoadCLIConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}
			
			// Show config source information
			configSource := getConfigSource(configPath)
			fmt.Printf("# Configuration loaded from: %s\n\n", configSource)
			
			// Display config in YAML format
			encoder := yaml.NewEncoder(os.Stdout)
			encoder.SetIndent(2)
			defer encoder.Close()
			if err := encoder.Encode(cfg); err != nil {
				return fmt.Errorf("failed to display config: %w", err)
			}
			
			fmt.Printf("\nTo edit: %s\n", outputPath)
			return nil
		case "2":
			// Continue with overwrite
			fmt.Println("Overwriting existing config file...")
		case "3":
			fmt.Println("Operation cancelled.")
			return nil
		default:
			fmt.Println("Invalid option. Operation cancelled.")
			return nil
		}
	}
	
	// Create example config
	if err := config.WriteExampleConfig(outputPath); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	
	fmt.Printf("Configuration file created: %s\n", outputPath)
	fmt.Println("Edit this file to customize your lambda-nat-proxy settings.")
	
	return nil
}

// runConfigShow implements the config show command
func runConfigShow(cmd *cobra.Command) error {
	// Load configuration
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadCLIConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	
	// Show config source information
	configSource := getConfigSource(configPath)
	fmt.Printf("# Configuration loaded from: %s\n\n", configSource)
	
	format, _ := cmd.Flags().GetString("format")
	
	switch format {
	case "yaml":
		encoder := yaml.NewEncoder(os.Stdout)
		encoder.SetIndent(2)
		defer encoder.Close()
		return encoder.Encode(cfg)
	case "json":
		// Could implement JSON output here
		return fmt.Errorf("JSON format not yet implemented")
	case "table":
		// Could implement table format here
		return fmt.Errorf("table format not yet implemented")
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// getConfigSource returns a user-friendly description of where config is loaded from
func getConfigSource(configPath string) string {
	if configPath != "" {
		// Explicit config file specified
		return configPath
	}
	
	// Check if config file exists in standard locations
	if foundPath, err := config.FindConfigFile(); err == nil {
		return foundPath
	}
	
	// No config file found, using defaults
	return "defaults (no config file found)"
}