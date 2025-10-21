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
		log.Fatal(err)
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

// ExampleDevEnvConfig_CPU demonstrates flexible CPU resource handling
// with different input types and default values.
func ExampleDevEnvConfig_CPU() {
	// Example with string CPU value
	cfg1 := &config.DevEnvConfig{
		Name: "alice",
		BaseConfig: config.BaseConfig{
			Resources: config.ResourceConfig{
				CPU: 8, // String value
			},
		},
	}
	fmt.Printf("String CPU: %s\n", cfg1.CPU())

	// Example with integer CPU value
	cfg2 := &config.DevEnvConfig{
		Name: "bob",
		BaseConfig: config.BaseConfig{
			Resources: config.ResourceConfig{
				CPU: 4, // Integer value
			},
		},
	}
	fmt.Printf("Integer CPU: %s\n", cfg2.CPU())

	// Example with no CPU specified (uses default)
	cfg3 := &config.DevEnvConfig{
		Name: "charlie",
		// No Resources specified
	}
	fmt.Printf("Default CPU: %s\n", cfg3.CPU())

	// Output:
	// String CPU: 8000m
	// Integer CPU: 4000m
	// Default CPU: 0
}

// ExampleDevEnvConfig_GetSSHKeys demonstrates SSH key handling with
// both single string and multiple string array formats.
func ExampleDevEnvConfig_GetSSHKeys() {
	// Single SSH key as string
	cfg1 := &config.DevEnvConfig{
		Name: "alice",
		BaseConfig: config.BaseConfig{
			SSHPublicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB alice@example.com",
		},
	}

	keys1, err := cfg1.GetSSHKeys()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Single key: %d SSH key(s)\n", len(keys1))

	// Multiple SSH keys as slice
	cfg2 := &config.DevEnvConfig{
		Name: "bob",
		BaseConfig: config.BaseConfig{
			SSHPublicKey: []interface{}{
				"ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB bob@work.com",
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI bob@home.com",
			},
		},
	}

	keys2, err := cfg2.GetSSHKeys()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Multiple keys: %d SSH key(s)\n", len(keys2))

	// Output:
	// Single key: 1 SSH key(s)
	// Multiple keys: 2 SSH key(s)
}
