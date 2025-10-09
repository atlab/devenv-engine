package config_test

import (
	"fmt"
	"log"

	"github.com/nauticalab/devenv-engine/internal/config"
)

// ExampleLoadDeveloperConfig demonstrates how to load a developer's configuration
// from a YAML file and access common configuration values.
func ExampleLoadDeveloperConfig() {
	// Load a developer's configuration
	cfg, err := config.LoadDeveloperConfig("testdata", "valid_user")
	if err != nil {
		log.Fatal(err)
	}

	// Access basic configuration
	fmt.Printf("Developer: %s\n", cfg.Name)
	fmt.Printf("CPU: %s\n", cfg.CPU())
	fmt.Printf("Memory: %s\n", cfg.Memory())
	fmt.Printf("User ID: %s\n", cfg.GetUserID())

	// Access SSH keys
	sshKeys, err := cfg.GetSSHKeys()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("SSH Keys: %d configured\n", len(sshKeys))

	// Output:
	// Developer: testuser
	// CPU: 4000m
	// Memory: 16Gi
	// User ID: 2000
	// SSH Keys: 1 configured
}
