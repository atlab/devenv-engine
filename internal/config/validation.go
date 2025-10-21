package config

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Package-level validator used by ValidateBaseConfig / ValidateDevEnvConfig.
var validate *validator.Validate

// sshKeyRE matches common OpenSSH public key formats:
//
//   - ssh-ed25519
//   - ssh-rsa
//   - ecdsa-sha2-nistp256 / nistp384 / nistp521
//   - sk-ecdsa-sha2-nistp256@openssh.com (FIDO)
//
// Pattern: <type><space><base64>[optional comment]
// Base64 is matched loosely with 0–2 '=' padding to accommodate real-world keys.
var sshKeyRegex = regexp.MustCompile(
	`^(?:(?:ssh-(?:ed25519|rsa))|(?:ecdsa-sha2-nistp(?:256|384|521))|(?:sk-ecdsa-sha2-nistp256@openssh\.com)) [A-Za-z0-9+/]+={0,2}(?: .+)?$`,
)

// numberRe matches a non-negative decimal number (integer or fractional).
// Examples: "0", "2", "2.5", "  3  ".
var numberRe = regexp.MustCompile(`^\s*[0-9]+(?:\.[0-9]+)?\s*$`)

// cpuMillicoresRe matches a non-negative decimal number with an 'm' suffix (millicores).
// Examples: "500m", "0m", "  12.5m  ".
var cpuMillicoresRe = regexp.MustCompile(`^\s*[0-9]+(?:\.[0-9]+)?m\s*$`)

// memoryRe matches Kubernetes-like memory quantities as strings, case-insensitive.
// Accepts:
//   - Binary: Ki, Mi, Gi, Ti, Pi, Ei
//   - Decimal SI: k, M, G, T, P, E
//   - Optional unit (bare numbers allowed — your parser treats these as Gi later)
//
// Examples: "512Mi", "16Gi", "500M", "1G", "1536", " 2.5Gi ".
var memoryRe = regexp.MustCompile(`(?i)^\s*[0-9]+(?:\.[0-9]+)?(?:ki|mi|gi|ti|pi|ei|k|m|g|t|p|e)?\s*$`)

func init() {
	// Enable "required on structs" semantics and register custom validators.
	validate = validator.New(validator.WithRequiredStructEnabled())

	if err := validate.RegisterValidation("ssh_keys", validateSSHKeys); err != nil {
		panic(fmt.Errorf("register validator ssh_keys: %w", err))
	}
	if err := validate.RegisterValidation("k8s_cpu", validateKubernetesCPU); err != nil {
		panic(fmt.Errorf("register validator k8s_cpu: %w", err))
	}
	if err := validate.RegisterValidation("k8s_memory", validateKubernetesMemory); err != nil {
		panic(fmt.Errorf("register validator k8s_memory: %w", err))
	}
}

// validateSSHKeys implements the "ssh_keys" tag.
// It normalizes the flexible field (nil | string | []string | []any of string) to []string,
// trims each entry, and validates format via sshKeyRE. It returns true iff all present
// entries are valid. Presence (≥1) is enforced separately in ValidateDevEnvConfig.
func validateSSHKeys(fl validator.FieldLevel) bool {
	sshKeyField := fl.Field().Interface()

	// Normalize to string slice first
	sshKeys, err := normalizeSSHKeys(sshKeyField)
	if err != nil {
		return false
	}
	for _, key := range sshKeys {
		key = strings.TrimSpace(key)
		if key == "" || !sshKeyRegex.MatchString(key) {
			return false
		}
	}
	return true
}

// validateKubernetesCPU implements the "k8s_cpu" tag for *raw* CPU fields.
// Accepts:
//   - Strings: "", "unlimited", plain number ("2", "2.5"), or millicores ("500m")
//   - Numbers (int/uint/float): non-negative
//
// Negatives and malformed strings are rejected.
// NOTE: canonicalization (→ millicores) happens in normalizeCPU during loading.
func validateKubernetesCPU(fl validator.FieldLevel) bool {
	cpuField := fl.Field().Interface()
	switch v := cpuField.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" || strings.EqualFold(s, "unlimited") {
			return true
		}
		// Check if it's a valid number or decimal
		if numberRe.MatchString(s) || cpuMillicoresRe.MatchString(s) {
			// Strip optional 'm' and parse number to ensure it's a valid float ≥ 0.
			if strings.HasSuffix(strings.ToLower(s), "m") {
				s = strings.TrimSpace(strings.TrimSuffix(s, "m"))
			}
			f, err := strconv.ParseFloat(s, 64)
			return err == nil && f >= 0
		}
		return false

	case int:
		return v >= 0
	case int64:
		return v >= 0

	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0

	default:
		return false
	}
}

// validateKubernetesMemory implements the "k8s_memory" tag for *raw* memory fields.
// Accepts:
//   - Strings: "", "unlimited", or a non-negative decimal + optional unit among
//     Ki/Mi/Gi/Ti/Pi/Ei (binary) or k/M/G/T/P/E (decimal SI), case-insensitive.
//     Bare numbers are allowed (your parser interprets them as Gi).
//   - Numbers (int/uint/float): non-negative
//
// Negatives and malformed strings are rejected.
// NOTE: canonicalization (→ MiB) happens in normalizeMemory during loading.
func validateKubernetesMemory(fl validator.FieldLevel) bool {
	switch v := fl.Field().Interface().(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" || strings.EqualFold(s, "unlimited") {
			return true
		}
		if !memoryRe.MatchString(s) {
			return false
		}
		// Strip known unit suffix (if any) before parsing the number.
		ls := strings.ToLower(s)
		for _, suf := range []string{"ki", "mi", "gi", "ti", "pi", "ei", "k", "m", "g", "t", "p", "e"} {
			if strings.HasSuffix(ls, suf) {
				s = s[:len(s)-len(suf)]
				break
			}
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return err == nil && f >= 0

	case int:
		return v >= 0

	case int64:
		return v >= 0

	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0

	default:
		return false
	}
}

// ValidateDevEnvConfig runs tag-based validation and then applies
// additional semantic checks that are easier to express in code.
func ValidateDevEnvConfig(config *DevEnvConfig) error {
	if err := validate.Struct(config); err != nil {
		return formatValidationError(err)
	}

	// Require ≥1 SSH public key with valid format.
	sshKeys, err := config.GetSSHKeys()
	if err != nil {
		return fmt.Errorf("invalid SSH public key(s): %w", err)
	}

	if len(sshKeys) == 0 {
		return fmt.Errorf("at least one SSH public key is required")
	}

	if mc, err := config.Resources.getCanonicalCPU(); err != nil || mc < 0 {
		return err // "cpu must be >= 0"
	}

	if mi, err := config.Resources.getCanonicalMemory(); err != nil || mi < 0 {
		return err // "memory must be >= 0"
	}

	if config.Resources.GPU < 0 {
		return fmt.Errorf("gpu must be >= 0")
	}

	return nil
}

// ValidateBaseConfig validates only the BaseConfig portion; useful for
// validating global defaults or partial configs before embedding.
func ValidateBaseConfig(config *BaseConfig) error {
	if err := validate.Struct(config); err != nil {
		return formatValidationError(err)
	}
	return nil
}

// formatValidationError renders go-playground/validator errors as concise, user-facing text.
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
	case "url":
		return fmt.Sprintf("'%s' must be a valid URL, got '%v'", fieldName, value)
	case "filepath":
		return fmt.Sprintf("'%s' must be a valid file path, got '%v'", fieldName, value)
	case "cron":
		return fmt.Sprintf("'%s' must be a valid cron expression, got '%v'", fieldName, value)

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
