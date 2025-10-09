package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	// Opt-in to v11+ behavior
	validate = validator.New(validator.WithRequiredStructEnabled())

	// Register custom validators
	validate.RegisterValidation("ssh_keys", validateSSHKeys)
	validate.RegisterValidation("k8s_cpu", validateKubernetesCPU)
	validate.RegisterValidation("k8s_memory", validateKubernetesMemory)
}

// validateSSHKeys validates SSH public key format
func validateSSHKeys(fl validator.FieldLevel) bool {
	sshKeyField := fl.Field().Interface()

	// Normalize to string slice first
	sshKeys, err := normalizeSSHKeys(sshKeyField)
	if err != nil {
		return false
	}

	// Validate each SSH key format
	sshKeyRegex := regexp.MustCompile(`^ssh-(rsa|ed25519|ecdsa) [A-Za-z0-9+/]+=*( .+)?$`)

	for _, key := range sshKeys {
		key = strings.TrimSpace(key)
		if key == "" || !sshKeyRegex.MatchString(key) {
			return false
		}
	}
	return true
}

func validateKubernetesCPU(fl validator.FieldLevel) bool {
	cpuField := fl.Field().Interface()
	switch cpu := cpuField.(type) {
	case string:
		if cpu == "" || cpu == "unlimited" {
			return true // Valid special values
		}
		// Check if it's a valid number or decimal
		if _, err := strconv.ParseFloat(cpu, 64); err != nil {
			// Check for 'm' suffix (millicores)
			if strings.HasSuffix(cpu, "m") {
				cpuValue := strings.TrimSuffix(cpu, "m")
				_, err := strconv.ParseFloat(cpuValue, 64)
				return err == nil
			}
			return false
		}
		return true
	case int:
		return cpu >= 0 // Non-negative integer
	case float64:
		return cpu >= 0 // Non-negative float
	default:
		return false // Invalid type
	}
}

func validateKubernetesMemory(fl validator.FieldLevel) bool {
	// get lowercase of string value
	memory := fl.Field().String()
	if memory == "" || strings.ToLower(memory) == "unlimited" {
		return true // Valid special values
	}

	// Kubernetes memory/straoge format: number + unit (Ki, Mi, Gi, Ti, Pi, Ei)
	memoryRegex := regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(Ki|Mi|Gi|Ti|Pi|Ei)?$`)
	return memoryRegex.MatchString(memory)
}

// ValidateDevEnvConfig validates a DevEnvConfig using the validator
func ValidateDevEnvConfig(config *DevEnvConfig) error {
	if err := validate.Struct(config); err != nil {
		return formatValidationError(err)
	}

	// Additional validation for BaseConfig fields that needs special handling
	sshKeys, err := config.GetSSHKeys()
	if err != nil {
		return fmt.Errorf("failed to get SSH keys: %w", err)
	}

	if len(sshKeys) == 0 {
		return fmt.Errorf("at least one SSH public key is required")
	}

	return nil
}

// ValidateBaseConfig validates a BaseConfig using the validator
func ValidateBaseConfig(config *BaseConfig) error {
	if err := validate.Struct(config); err != nil {
		return formatValidationError(err)
	}
	return nil
}

// formatValidationError converts validator errors to user-friendly messages
func formatValidationError(err error) error {
	var errorMessages []string

	validationErrors := err.(validator.ValidationErrors)
	for _, fieldError := range validationErrors {
		message := formatFieldError(fieldError)
		errorMessages = append(errorMessages, message)
	}

	return fmt.Errorf("configuration validation failed:\n  - %s",
		strings.Join(errorMessages, "\n  - "))
}

// formatFieldError creates user-friendly error messages for field validation failures
func formatFieldError(fieldError validator.FieldError) string {
	fieldName := fieldError.Field()
	tag := fieldError.Tag()
	param := fieldError.Param()
	value := fieldError.Value()

	switch tag {
	case "required":
		return fmt.Sprintf("'%s' is required", fieldName)
	case "email":
		return fmt.Sprintf("'%s' must be a valid email address, got '%v'", fieldName, value)
	case "min":
		return fmt.Sprintf("'%s' must be at least %s characters/value, got '%v'", fieldName, param, value)
	case "max":
		return fmt.Sprintf("'%s' must be at most %s characters/value, got '%v'", fieldName, param, value)
	case "hostname":
		return fmt.Sprintf("'%s' must be a valid hostname format, got '%v'", fieldName, value)
	case "ssh_keys":
		return fmt.Sprintf("'%s' contains invalid SSH key format", fieldName)
	case "k8s_cpu":
		return fmt.Sprintf("'%s' must be a valid Kubernetes CPU format (e.g., '2', '1.5', '500m'), got '%v'", fieldName, value)
	case "k8s_memory":
		return fmt.Sprintf("'%s' must be a valid Kubernetes memory format (e.g., '1Gi', '512Mi'), got '%v'", fieldName, value)
	default:
		return fmt.Sprintf("'%s' failed validation '%s', got '%v'", fieldName, tag, value)
	}
}
