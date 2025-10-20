package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadGlobalConfig loads the global configuration file (devenv.yaml) from the config directory.
// Returns a BaseConfig pre-populated with system defaults. If the global config file exists,
// YAML values override the defaults. If the file doesn't exist, returns defaults without error.
func LoadGlobalConfig(configDir string) (*BaseConfig, error) {
	globalConfigPath := filepath.Join(configDir, "devenv.yaml")

	// Start with system defaults
	globalConfig := NewBaseConfigWithDefaults()

	// Check if global config file exists
	if _, err := os.Stat(globalConfigPath); os.IsNotExist(err) {
		return &globalConfig, nil // Return defaults if file doesn't exist
	}

	// Read the global config file
	data, err := os.ReadFile(globalConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read global config file %s: %w", globalConfigPath, err)
	}

	// Unmarshal into pre-populated struct - only overrides present fields
	if err := yaml.Unmarshal(data, &globalConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in global config %s: %w", globalConfigPath, err)
	}

	// Normalize CPU (raw may be string/int/float/nil)
	if err := normalizeCPU(&globalConfig.Resources); err != nil {
		return nil, fmt.Errorf(
			"failed to normalize CPU in %q (cpu=%v): %w",
			globalConfigPath, globalConfig.Resources.CPURaw, err,
		)
	}

	// Normalize Memory (raw may be string/int/float/nil)
	if err := normalizeMemory(&globalConfig.Resources); err != nil {
		return nil, fmt.Errorf(
			"failed to normalize memory in %q (memory=%v): %w",
			globalConfigPath, globalConfig.Resources.MemoryRaw, err,
		)
	}
	return &globalConfig, nil
}

// LoadDeveloperConfig loads and parses a developer's configuration file
// from the specified directory. It reads the devenv-config.yaml file from
// the developer's subdirectory, validates the configuration, and returns
// a populated DevEnvConfig struct with only basic validation.
//
// This function does NOT merge with global defaults - use LoadDeveloperConfigWithGlobalDefaults
// for that functionality.
func LoadDeveloperConfig(configDir, developerName string) (*DevEnvConfig, error) {
	developerDir := filepath.Join(configDir, developerName)
	configPath := filepath.Join(developerDir, "devenv-config.yaml")

	// Check if the config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configPath)
	}

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Create empty config (no defaults)
	var config DevEnvConfig

	// Parse the YAML
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in %s: %w", configPath, err)
	}

	config.DeveloperDir = developerDir

	// Normalize flexible → canonical
	if err := normalizeCPU(&config.Resources); err != nil {
		return nil, fmt.Errorf(
			"failed to normalize CPU in %q (cpu=%v): %w",
			configPath, config.Resources.CPURaw, err,
		)
	}
	if err := normalizeMemory(&config.Resources); err != nil {
		return nil, fmt.Errorf(
			"failed to normalize memory in %q (memory=%v): %w",
			configPath, config.Resources.MemoryRaw, err,
		)
	}

	// Basic validation
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration in %s: %w", configPath, err)
	}

	return &config, nil
}

// LoadDeveloperConfigWithGlobalDefaults loads a developer config and merges it with global defaults.
// This is the recommended loading function that provides the complete configuration hierarchy:
// System defaults → Global config → User config
func LoadDeveloperConfigWithBaseConfig(configDir, developerName string, baseConfig *BaseConfig) (*DevEnvConfig, error) {

	// Step 2: Create user config pre-populated with global config values
	userConfig := &DevEnvConfig{
		BaseConfig: *baseConfig, // Copy all global values (which include system defaults)
	}

	// Step 3: Load user YAML
	developerDir := filepath.Join(configDir, developerName)
	configPath := filepath.Join(developerDir, "devenv-config.yaml")

	// Check if the config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configPath)
	}

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Step 4: Unmarshal user YAML - overwrites only fields present in YAML
	if err := yaml.Unmarshal(data, userConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in %s: %w", configPath, err)
	}

	// Step 5: Merge additive list fields (packages, volumes, SSH keys)
	// Note that this step is neceessary because YAML unmarshaling replaces slices
	userConfig.mergeListFields(baseConfig)

	// Step 6: Set developer directory and validate
	userConfig.DeveloperDir = developerDir

	// Step 7: Normalize flexible/raw fields → canonical representation
	// Normalize flexible → canonical
	if err := normalizeCPU(&userConfig.Resources); err != nil {
		return nil, fmt.Errorf(
			"failed to normalize CPU in %q (cpu=%v): %w",
			configPath, userConfig.Resources.CPURaw, err,
		)
	}
	if err := normalizeMemory(&userConfig.Resources); err != nil {
		return nil, fmt.Errorf(
			"failed to normalize memory in %q (memory=%v): %w",
			configPath, userConfig.Resources.MemoryRaw, err,
		)
	}

	if err := userConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration in %s: %w", configPath, err)
	}

	return userConfig, nil
}

// mergeListFields handles additive merging for packages, volumes, and SSH keys
func (config *DevEnvConfig) mergeListFields(globalConfig *BaseConfig) {
	// Save current user values before merging
	userPackagesPython := config.Packages.Python
	userPackagesAPT := config.Packages.APT
	userVolumes := config.Volumes

	// Merge packages: global packages + user packages
	config.Packages.Python = mergeStringSlices(globalConfig.Packages.Python, userPackagesPython)
	config.Packages.APT = mergeStringSlices(globalConfig.Packages.APT, userPackagesAPT)

	// Merge volumes: global volumes + user volumes
	config.Volumes = mergeVolumes(globalConfig.Volumes, userVolumes)

	// Merge SSH keys: global SSH keys + user SSH keys
	globalSSHKeys, err := globalConfig.GetSSHKeys()
	if err != nil {
		globalSSHKeys = []string{}
	}

	userSSHKeys, err := config.GetSSHKeys()
	if err != nil {
		userSSHKeys = []string{}
	}

	mergedSSHKeys := mergeStringSlices(globalSSHKeys, userSSHKeys)
	config.SSHPublicKey = mergedSSHKeys
}

// ============================================================================
// Utility functions for configuration merging and normalization
// ============================================================================

// mergeStringSlices combines two string slices, removing duplicates
// The global slice items come first, followed by user slice items
func mergeStringSlices(global, user []string) []string {
	if len(global) == 0 {
		return user
	}
	if len(user) == 0 {
		return global
	}

	// Use map to track seen values and maintain order
	seen := make(map[string]bool)
	var result []string

	// Add global values first
	for _, item := range global {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	// Add user values, skipping duplicates
	for _, item := range user {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// mergeVolumes combines global and user volume mounts
// User volumes with the same name override global volumes
func mergeVolumes(global, user []VolumeMount) []VolumeMount {
	if len(global) == 0 {
		return user
	}
	if len(user) == 0 {
		return global
	}

	// Create map of user volumes by name for quick lookup
	userVolumesByName := make(map[string]VolumeMount)
	for _, vol := range user {
		userVolumesByName[vol.Name] = vol
	}

	var result []VolumeMount

	// Add global volumes, but skip if user has same name
	for _, globalVol := range global {
		if _, exists := userVolumesByName[globalVol.Name]; !exists {
			result = append(result, globalVol)
		}
	}

	// Add all user volumes
	result = append(result, user...)

	return result
}

// normalizeSSHKeys converts the flexible SSH key field to a string slice
// Handles both single string and string array formats from YAML
func normalizeSSHKeys(sshKeyField any) ([]string, error) {
	if sshKeyField == nil {
		return []string{}, nil
	}

	switch keys := sshKeyField.(type) {
	case string:
		s := strings.TrimSpace(keys)
		// Single SSH key
		if s == "" {
			return []string{}, fmt.Errorf("SSH key cannot be empty string")
		}
		return []string{s}, nil

	case []string:
		// Direct string slice
		if len(keys) == 0 {
			return nil, fmt.Errorf("SSH key array cannot be empty")
		}
		out := make([]string, len(keys))
		for i, k := range keys {
			s := strings.TrimSpace(k)
			if s == "" {
				return nil, fmt.Errorf("SSH key at index %d cannot be empty", i)
			}
			out[i] = s
		}
		return out, nil

	case []any: // alias of []interface{}
		if len(keys) == 0 {
			return nil, fmt.Errorf("SSH key array cannot be empty")
		}
		out := make([]string, len(keys))
		// Array of SSH keys (from YAML)
		for i, e := range keys {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("SSH key at index %d is not a string", i)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return nil, fmt.Errorf("SSH key at index %d cannot be empty", i)
			}
			out[i] = s
		}
		return out, nil

	default:
		return nil, fmt.Errorf("SSH key field must be string or array of strings, got %T", sshKeyField)
	}
}

// normalizeCPU reads r.CPURaw and sets r.CPU (millicores).
// With strict policy, invalid or non-positive inputs propagate as zero with an error, or
// you can choose to return ok=false and let the loader decide. Here I return an error.
func normalizeCPU(r *ResourceConfig) error {
	if r.CPURaw == nil {
		return nil // no user value; keep default already in r.CPU
	}
	m, ok := toMillicores(r.CPURaw)
	if !ok {
		// make policy explicit: treat invalid as an error
		return fmt.Errorf("invalid cpu value: %v", r.CPURaw)
	}
	r.CPU = m
	return nil
}

// toMillicores converts a flexible CPU value to millicores.
// Policy: syntactically valid but non-positive (<=0) values are INVALID → ok=false.
func toMillicores(v any) (int64, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false

	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false
		}
		// Already in millicores: "500m"
		if strings.HasSuffix(s, "m") {
			d := strings.TrimSpace(strings.TrimSuffix(s, "m"))
			if !allDigits(d) {
				return 0, false
			}
			n, err := strconv.ParseInt(d, 10, 64)
			if err != nil || n <= 0 {
				return 0, false
			}
			return n, true
		}
		// Plain number: "2", "2.5"
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f <= 0 {
			return 0, false
		}
		return int64(math.Round(f * 1000.0)), true

	// Signed ints: cores
	case int:
		if x <= 0 {
			return 0, false
		}
		return int64(x) * 1000, true
	case int8:
		if x <= 0 {
			return 0, false
		}
		return int64(x) * 1000, true
	case int16:
		if x <= 0 {
			return 0, false
		}
		return int64(x) * 1000, true
	case int32:
		if x <= 0 {
			return 0, false
		}
		return int64(x) * 1000, true
	case int64:
		if x <= 0 {
			return 0, false
		}
		return x * 1000, true

	// Unsigned ints: cores (cannot be negative)
	case uint:
		if x == 0 {
			return 0, false
		}
		return int64(x) * 1000, true
	case uint8:
		if x == 0 {
			return 0, false
		}
		return int64(x) * 1000, true
	case uint16:
		if x == 0 {
			return 0, false
		}
		return int64(x) * 1000, true
	case uint32:
		if x == 0 {
			return 0, false
		}
		return int64(x) * 1000, true
	case uint64:
		if x == 0 || x > math.MaxInt64/1000 {
			return 0, false
		}
		return int64(x) * 1000, true

	// Floats (cores)
	case float32:
		f := float64(x)
		if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return int64(math.Round(f * 1000.0)), true
	case float64:
		if x <= 0 || math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		return int64(math.Round(x * 1000.0)), true

	default:
		return 0, false
	}
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// normalizeMemory reads r.MemoryRaw (e.g., "16Gi", "512Mi", "500M", "1.5", 2, …)
// and sets r.Memory to Mi (mebibytes). Invalid or non-positive → 0.
func normalizeMemory(r *ResourceConfig) error {
	if r.MemoryRaw == nil {
		return nil // no user value; keep default already in r.Memory
	}
	mi, ok := toMi(r.MemoryRaw)
	if !ok || mi <= 0 {
		r.Memory = 0
		return nil
	}
	r.Memory = mi
	return nil
}

// toMi converts a flexible memory value into canonical Mi (mebibytes).
// Strict policy: syntactically valid but non-positive (<= 0) values are INVALID (ok=false).
func toMi(v any) (int64, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false

	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false
		}
		return parseStringToMi(s)

	// Signed ints: interpret as Gi
	case int:
		if x <= 0 {
			return 0, false
		}
		if int64(x) > math.MaxInt64/1024 {
			return 0, false
		}
		return int64(x) * 1024, true
	case int8:
		if x <= 0 {
			return 0, false
		}
		return int64(x) * 1024, true
	case int16:
		if x <= 0 {
			return 0, false
		}
		return int64(x) * 1024, true
	case int32:
		if x <= 0 {
			return 0, false
		}
		return int64(x) * 1024, true
	case int64:
		if x <= 0 || x > math.MaxInt64/1024 {
			return 0, false
		}
		return x * 1024, true

	// Unsigned ints: interpret as Gi (guard overflow and zero)
	case uint:
		if x == 0 || uint64(x) > uint64(math.MaxInt64/1024) {
			return 0, false
		}
		return int64(x) * 1024, true
	case uint8:
		if x == 0 {
			return 0, false
		}
		return int64(x) * 1024, true
	case uint16:
		if x == 0 {
			return 0, false
		}
		return int64(x) * 1024, true
	case uint32:
		if x == 0 {
			return 0, false
		}
		return int64(x) * 1024, true
	case uint64:
		if x == 0 || x > uint64(math.MaxInt64/1024) {
			return 0, false
		}
		return int64(x) * 1024, true

	// Floats: interpret as Gi
	case float32:
		return roundGiToMi(float64(x))
	case float64:
		return roundGiToMi(x)

	default:
		return 0, false
	}
}

// roundGiToMi converts Gi (float) to Mi with rounding.
// Returns (0,false) if gi <= 0, NaN/Inf, or rounding yields 0 Mi.
func roundGiToMi(gi float64) (int64, bool) {
	if gi <= 0 || math.IsNaN(gi) || math.IsInf(gi, 0) {
		return 0, false
	}
	miFloat := gi * 1024.0
	mi := int64(math.Round(miFloat))
	if mi <= 0 {
		return 0, false
	}
	return mi, true
}

// parseStringToMi handles binary units (Ki/Mi/Gi/Ti/Pi/Ei),
// decimal SI (k/M/G/T/P/E), and bare numeric strings (Gi).
// Returns (0,false) if value <= 0 or on invalid input.
func parseStringToMi(s string) (int64, bool) {
	// ---- Binary SI (powers of 1024) ----
	if hasSuffixFold(s, "Ki") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-2]))
		if !ok || n <= 0 {
			return 0, false
		}
		// KiB → MiB
		miFloat := n / 1024.0
		mi := int64(math.Round(miFloat))
		if mi <= 0 {
			return 0, false
		}
		return mi, true
	}
	if hasSuffixFold(s, "Mi") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-2]))
		if !ok || n <= 0 {
			return 0, false
		}
		mi := int64(math.Round(n))
		if mi <= 0 {
			return 0, false
		}
		return mi, true
	}
	if hasSuffixFold(s, "Gi") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-2]))
		if !ok || n <= 0 {
			return 0, false
		}
		return roundGiToMi(n)
	}
	if hasSuffixFold(s, "Ti") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-2]))
		if !ok || n <= 0 {
			return 0, false
		}
		miFloat := n * 1024.0 * 1024.0
		if miFloat > float64(math.MaxInt64) {
			return 0, false
		}
		mi := int64(math.Round(miFloat))
		if mi <= 0 {
			return 0, false
		}
		return mi, true
	}
	if hasSuffixFold(s, "Pi") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-2]))
		if !ok || n <= 0 {
			return 0, false
		}
		miFloat := n * 1024.0 * 1024.0 * 1024.0
		if miFloat > float64(math.MaxInt64) {
			return 0, false
		}
		mi := int64(math.Round(miFloat))
		if mi <= 0 {
			return 0, false
		}
		return mi, true
	}
	if hasSuffixFold(s, "Ei") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-2]))
		if !ok || n <= 0 {
			return 0, false
		}
		// Ei → Mi (giant); guard overflow
		miFloat := n * 1024.0 * 1024.0 * 1024.0 * 1024.0
		if miFloat > float64(math.MaxInt64) {
			return 0, false
		}
		mi := int64(math.Round(miFloat))
		if mi <= 0 {
			return 0, false
		}
		return mi, true
	}

	// ---- Decimal SI (powers of 1000) ----
	if hasSuffixFold(s, "k") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-1]))
		if !ok || n <= 0 {
			return 0, false
		}
		return bytesToMi(n * 1e3)
	}
	if hasSuffixFold(s, "M") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-1]))
		if !ok || n <= 0 {
			return 0, false
		}
		return bytesToMi(n * 1e6)
	}
	if hasSuffixFold(s, "G") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-1]))
		if !ok || n <= 0 {
			return 0, false
		}
		return bytesToMi(n * 1e9)
	}
	if hasSuffixFold(s, "T") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-1]))
		if !ok || n <= 0 {
			return 0, false
		}
		return bytesToMi(n * 1e12)
	}
	if hasSuffixFold(s, "P") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-1]))
		if !ok || n <= 0 {
			return 0, false
		}
		return bytesToMi(n * 1e15)
	}
	if hasSuffixFold(s, "E") {
		n, ok := parseNumber(strings.TrimSpace(s[:len(s)-1]))
		if !ok || n <= 0 {
			return 0, false
		}
		return bytesToMi(n * 1e18)
	}

	// ---- Bare number: interpret as Gi (not bytes) ----
	if n, ok := parseNumber(s); ok && n > 0 {
		return roundGiToMi(n)
	}
	return 0, false
}

// bytesToMi converts decimal bytes to Mi with rounding.
// Returns (0,false) if bytes <= 0, NaN/Inf, or if the result would overflow or round to 0.
func bytesToMi(bytes float64) (int64, bool) {
	if bytes <= 0 || math.IsNaN(bytes) || math.IsInf(bytes, 0) {
		return 0, false
	}
	miFloat := bytes / (1024.0 * 1024.0)
	if miFloat > float64(math.MaxInt64) {
		return 0, false
	}
	mi := int64(math.Round(miFloat))
	if mi <= 0 {
		return 0, false
	}
	return mi, true
}

// parseNumber accepts integers/decimals/scientific notation (e.g., "1e6").
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// Case-insensitive suffix check.
func hasSuffixFold(s, suf string) bool {
	return len(s) >= len(suf) && strings.EqualFold(s[len(s)-len(suf):], suf)
}
