package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nauticalab/devenv-engine/internal/config"
	"github.com/nauticalab/devenv-engine/internal/templates"
	"github.com/spf13/cobra"
)

// DeveloperJob represents work to be done for one developer
type DeveloperJob struct {
	Name string
}

// ProcessingResult represents the outcome of processing one developer
type ProcessingResult struct {
	Developer string
	Success   bool
	Error     error
	Duration  time.Duration
}

var (
	// Command-specific flags for generate
	outputDir string
	configDir string // Input directory for developer configs
	dryRun    bool
	allDevs   bool
)

var generateCmd = &cobra.Command{
	Use:   "generate [developer-name]",
	Short: "Generate manifests for a developer environment",
	Long: `Generate Kubernetes manifests for a specific developer or all developers.

Examples:
  devenv generate eywalker
  devenv generate --all-developers --output ./manifests`,
	Args: cobra.MaximumNArgs(1), // At max 1 argument
	Run: func(cmd *cobra.Command, args []string) {
		//Validation logic
		if allDevs && len(args) > 0 {
			fmt.Fprintf(os.Stderr, "error: Cannot specify developer name with --all-developers flag\n")
			os.Exit(1)
		}

		if !allDevs && len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: Please specify a developer name or use --all-developers\n")
			cmd.Help()
			os.Exit(1)
		}

		// Execute the logic (placeholder for now)
		if allDevs {
			fmt.Println("Generating manifests for all developers...")
			if verbose {
				fmt.Printf("Output directory: %s\n", outputDir)
			}
			generateAllDevelopersWithProgress()
		} else {
			developerName := args[0]
			generateSingleDeveloper(developerName)
		}
	},
}

func init() {
	// Generate command specific flags
	generateCmd.Flags().StringVarP(&outputDir, "output", "o", "./build", "Output directory for generated manifests")
	generateCmd.Flags().StringVar(&configDir, "config-dir", "./developers", "Directory containing developer configuration files")
	generateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be generated without creating files")
	generateCmd.Flags().BoolVar(&allDevs, "all-developers", false, "Generate manifests for all developers")

}

func generateAllDevelopersWithProgress() {
	// Step 1: Load global config once
	globalConfig, err := config.LoadGlobalConfig(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config in %s: %v\n", configDir, err)
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("Generating system manifests in %s\n", outputDir)
	}

	// Step 2: Generate system manifests once
	if !dryRun {
		if err := generateSystemManifests(globalConfig, outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating system manifests: %v\n", err)
			os.Exit(1)
		}
	}

	// Step 3: Discover all developers
	developers, err := findAllDevelopers(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering developers: %v\n", err)
		os.Exit(1)
	}

	if len(developers) == 0 {
		fmt.Printf("No developers found in %s\n", configDir)
		return
	}

	fmt.Printf("Found %d developers to process.\n", len(developers))

	// Step 4: Set up channels for worker communication
	const numWorkers = 4
	jobs := make(chan DeveloperJob, len(developers))
	results := make(chan ProcessingResult, len(developers))

	// Step 5: Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		go developerWorker(jobs, results, globalConfig)
	}

	// Step 6: Send all jobs to workers
	for _, dev := range developers {
		jobs <- DeveloperJob{Name: dev}
	}
	close(jobs)

	// Step 7: Collect results
	var successCount, failureCount int
	var failures []ProcessingResult

	for i := 0; i < len(developers); i++ {
		result := <-results
		if result.Success {
			successCount++
			fmt.Printf("[%d/%d] ✅ %s (%.1fs)\n",
				i+1, len(developers), result.Developer, result.Duration.Seconds())
		} else {
			failureCount++
			failures = append(failures, result)
			fmt.Printf("[%d/%d] ❌ %s (%.1fs): %v\n",
				i+1, len(developers), result.Developer, result.Duration.Seconds(), result.Error)
		}
	}

	// Step 8: Print final summary
	fmt.Printf("\n🎉 Batch processing complete!\n")
	fmt.Printf("✅ Successful: %d\n", successCount)
	if failureCount > 0 {
		fmt.Printf("❌ Failed: %d\n", failureCount)
	}

	if failureCount > 0 {
		fmt.Printf("\nFailures:\n")
		for _, failure := range failures {
			fmt.Printf("  - %s: %v\n", failure.Developer, failure.Error)
		}
		os.Exit(1) // Exit with error if any failures
	}
}

func developerWorker(jobs <-chan DeveloperJob, results chan<- ProcessingResult, globalConfig *config.BaseConfig) {
	for job := range jobs {
		startTime := time.Now()
		err := processSingleDeveloperForBatchWithError(job.Name, globalConfig)

		results <- ProcessingResult{
			Developer: job.Name,
			Success:   err == nil,
			Error:     err,
			Duration:  time.Since(startTime),
		}
	}
}

// processSingleDeveloperForBatchWithError processes a single developer for batch mode
func processSingleDeveloperForBatchWithError(developerName string, globalConfig *config.BaseConfig) error {
	if verbose {
		fmt.Printf("Processing developer: %s\n", developerName)
	}

	cfg, err := config.LoadDeveloperConfigWithBaseConfig(configDir, developerName, globalConfig)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create user-specific output directory
	userOutputDir := filepath.Join(outputDir, developerName)

	if !dryRun {
		if err := generateDeveloperManifests(cfg, userOutputDir); err != nil {
			return fmt.Errorf("failed to generate manifests: %w", err)
		}
	}

	return nil
}

func findAllDevelopers(configDir string) ([]string, error) {
	var developers []string

	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read config directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check to make sure devenv-config.yaml exists in this directory
			configPath := filepath.Join(configDir, entry.Name(), "devenv-config.yaml")
			if _, err := os.Stat(configPath); err == nil {
				developers = append(developers, entry.Name())
			}
		}
	}

	return developers, nil
}

// generateSingleDeveloper handles generation for a single developer
func generateSingleDeveloper(developerName string) {
	fmt.Printf("Generating manifests for developer: %s\n", developerName)

	if verbose {
		fmt.Printf("Output directory: %s\n", outputDir)
		fmt.Printf("Config directory: %s\n", configDir)
		fmt.Printf("Dry run mode: %t\n", dryRun)
	}

	userOutputDir := filepath.Join(outputDir, developerName)

	globalConfig, err := config.LoadGlobalConfig(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading global config in %s: %v\n", configDir, err)
		os.Exit(1)
	}

	if err := generateSystemManifests(globalConfig, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating system manifests: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.LoadDeveloperConfigWithBaseConfig(configDir, developerName, globalConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config for developer %s: %v\n", developerName, err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully loaded configuration for developer: %s\n", cfg.Name)

	if verbose {
		printConfigSummary(cfg)
	}

	if !dryRun {
		if err := generateDeveloperManifests(cfg, userOutputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating manifests: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("🔍 Dry run - would generate manifests to: %s\n", userOutputDir)
	}
}

func generateSystemManifests(cfg *config.BaseConfig, outputDir string) error {
	// Create template renderer
	renderer := templates.NewSystemRenderer(outputDir)

	// Render all main templates
	if err := renderer.RenderAll(cfg); err != nil {
		return fmt.Errorf("failed to render templates: %w", err)
	}

	fmt.Printf("🎉 Successfully generated system manifests\n")

	return nil
}

// generateDeveloperManifests creates Kubernetes manifests for a developer
func generateDeveloperManifests(cfg *config.DevEnvConfig, outputDir string) error {
	// Create template renderer
	renderer := templates.NewDevRenderer(outputDir)

	// Render all main templates
	if err := renderer.RenderAll(cfg); err != nil {
		return fmt.Errorf("failed to render templates: %w", err)
	}

	fmt.Printf("🎉 Successfully generated manifests for %s\n", cfg.Name)

	return nil
}

// Helper function to print config summary
func printConfigSummary(cfg *config.DevEnvConfig) {
	fmt.Printf("\nConfiguration Summary:\n")
	fmt.Printf("  Name: %s\n", cfg.Name)

	sshKeys, _ := cfg.GetSSHKeys()
	fmt.Printf("  SSH Keys: %d configured\n", len(sshKeys))

	if cfg.SSHPort != 0 {
		fmt.Printf("  SSH Port: %d\n", cfg.SSHPort)
	}

	if cfg.Git.Name != "" {
		fmt.Printf("  Git: %s <%s>\n", cfg.Git.Name, cfg.Git.Email)
	}

	cpuStr := cfg.CPU()    // e.g., "4000m" or "0"
	memStr := cfg.Memory() // e.g., "16Gi" or ""

	hasCPU := cpuStr != "0"
	hasMem := memStr != ""

	if hasCPU || hasMem {
		fmt.Printf("  Resources:")
		if hasCPU {
			fmt.Printf(" CPU=%s", cpuStr)
		}
		if hasMem {
			if hasCPU {
				fmt.Printf(",")
			}
			fmt.Printf(" Memory=%s", memStr)
		}
		fmt.Println()
	}

	if len(cfg.Volumes) > 0 {
		fmt.Printf("  Volumes: %d configured\n", len(cfg.Volumes))
	}

	fmt.Printf("  Developer Config Dir: %s\n", cfg.GetDeveloperDir())
}

// Helper function to format CPU value for display
func formatCPU(cpu any) string {
	if cpu == nil {
		return "default"
	}
	switch v := cpu.(type) {
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%v", v) // Fallback
	}
}
