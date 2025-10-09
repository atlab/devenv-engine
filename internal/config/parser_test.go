package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGlobalConfig(t *testing.T) {
	t.Run("global config file exists", func(t *testing.T) {
		// Create temp directory and global config file
		tempDir := t.TempDir()
		globalConfigPath := filepath.Join(tempDir, "devenv.yaml")

		globalConfigYAML := `image: "custom:latest"
installHomebrew: false
packages:
  apt: ["curl", "git"]
  python: ["requests"]
resources:
  cpu: 4
  memory: "16Gi"
`
		require.NoError(t, os.WriteFile(globalConfigPath, []byte(globalConfigYAML), 0o644))

		// Load global config
		cfg, err := LoadGlobalConfig(tempDir)
		require.NoError(t, err)

		// YAML values override defaults
		assert.Equal(t, "custom:latest", cfg.Image)
		assert.False(t, cfg.InstallHomebrew) // override default true

		// Canonical resource units: CPU in millicores, Memory in Mi
		assert.Equal(t, int64(4000), cfg.Resources.CPU)       // 4 cores -> 4000m
		assert.Equal(t, int64(16*1024), cfg.Resources.Memory) // 16Gi -> 16384 Mi

		// Also verify formatted getters via DevEnvConfig wrapper
		dev := &DevEnvConfig{BaseConfig: *cfg}
		assert.Equal(t, "4000m", dev.CPU())
		assert.Equal(t, "16Gi", dev.Memory())

		// Packages merged from YAML
		assert.Equal(t, []string{"curl", "git"}, cfg.Packages.APT)
		assert.Equal(t, []string{"requests"}, cfg.Packages.Python)

		// Unspecified fields keep defaults
		assert.False(t, cfg.ClearLocalPackages)
		assert.False(t, cfg.ClearVSCodeCache)
		assert.Equal(t, "/opt/venv/bin", cfg.PythonBinPath)
		assert.Equal(t, 1000, cfg.UID)
		assert.Equal(t, "20Gi", cfg.Resources.Storage) // default storage unchanged
		assert.Equal(t, 0, cfg.Resources.GPU)          // default GPU unchanged
	})

	t.Run("global config file does not exist -> system defaults", func(t *testing.T) {
		tempDir := t.TempDir()

		cfg, err := LoadGlobalConfig(tempDir)
		require.NoError(t, err)

		// Top-level defaults
		assert.Equal(t, "ubuntu:22.04", cfg.Image)
		assert.True(t, cfg.InstallHomebrew)
		assert.False(t, cfg.ClearLocalPackages)
		assert.False(t, cfg.ClearVSCodeCache)
		assert.Equal(t, "/opt/venv/bin", cfg.PythonBinPath)
		assert.Equal(t, 1000, cfg.UID)

		// Canonical resource defaults (CPU millicores, Memory Mi)
		assert.Equal(t, int64(2000), cfg.Resources.CPU)      // 2 cores
		assert.Equal(t, int64(8*1024), cfg.Resources.Memory) // 8Gi
		assert.Equal(t, "20Gi", cfg.Resources.Storage)
		assert.Equal(t, 0, cfg.Resources.GPU)

		// Slices are non-nil and empty
		assert.NotNil(t, cfg.Packages.APT)
		assert.Len(t, cfg.Packages.APT, 0)
		assert.NotNil(t, cfg.Packages.Python)
		assert.Len(t, cfg.Packages.Python, 0)
		assert.NotNil(t, cfg.Volumes)
		assert.Len(t, cfg.Volumes, 0)
	})

	t.Run("invalid YAML in global config -> error", func(t *testing.T) {
		tempDir := t.TempDir()
		globalConfigPath := filepath.Join(tempDir, "devenv.yaml")

		invalidYAML := "image: \"test\ninstallHomebrew: [invalid"
		require.NoError(t, os.WriteFile(globalConfigPath, []byte(invalidYAML), 0o644))

		_, err := LoadGlobalConfig(tempDir)
		require.Error(t, err)
		// Keep the substring check loose to avoid overfitting exact wording
		assert.Contains(t, strings.ToLower(err.Error()), "parse")
	})
}

func TestLoadDeveloperConfig(t *testing.T) {
	t.Run("valid developer config", func(t *testing.T) {
		tempDir := t.TempDir()
		developerDir := filepath.Join(tempDir, "alice")
		require.NoError(t, os.MkdirAll(developerDir, 0o755))

		configPath := filepath.Join(developerDir, "devenv-config.yaml")
		// Include resources to exercise normalization to canonical units.
		configYAML := `name: alice
sshPublicKey:
  - "ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB alice@example.com"
sshPort: 30022
isAdmin: true
git:
  name: "Alice Smith"
  email: "alice@example.com"
packages:
  python: ["numpy", "pandas"]
  apt: ["vim"]
resources:
  cpu: 4
  memory: "16Gi"
`
		require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0o644))

		// Load developer config
		cfg, err := LoadDeveloperConfig(tempDir, "alice")
		require.NoError(t, err)

		// Basic fields
		assert.Equal(t, "alice", cfg.Name)
		assert.Equal(t, 30022, cfg.SSHPort)
		assert.True(t, cfg.IsAdmin)
		assert.Equal(t, "Alice Smith", cfg.Git.Name)
		assert.Equal(t, "alice@example.com", cfg.Git.Email)
		assert.Equal(t, []string{"numpy", "pandas"}, cfg.Packages.Python)
		assert.Equal(t, []string{"vim"}, cfg.Packages.APT)

		// Canonical resources: CPU millicores, Memory Mi
		assert.Equal(t, int64(4000), cfg.Resources.CPU)       // 4 cores → 4000m
		assert.Equal(t, int64(16*1024), cfg.Resources.Memory) // 16Gi → 16384 Mi

		// Getter formatting (K8s quantities)
		assert.Equal(t, "4000m", cfg.CPU())
		assert.Equal(t, "16Gi", cfg.Memory())

		// SSH keys (strict accessor)
		keys, err := cfg.GetSSHKeys()
		require.NoError(t, err)
		assert.Equal(t, []string{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB alice@example.com"}, keys)

		// DeveloperDir set
		assert.Equal(t, developerDir, cfg.DeveloperDir)
	})

	t.Run("config file not found", func(t *testing.T) {
		tempDir := t.TempDir()

		_, err := LoadDeveloperConfig(tempDir, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "configuration file not found")
	})

	t.Run("invalid config - missing SSH key", func(t *testing.T) {
		tempDir := t.TempDir()
		developerDir := filepath.Join(tempDir, "alice")
		require.NoError(t, os.MkdirAll(developerDir, 0o755))

		configPath := filepath.Join(developerDir, "devenv-config.yaml")
		configYAML := `name: alice`
		require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0o644))

		_, err := LoadDeveloperConfig(tempDir, "alice")
		require.Error(t, err)
		// Validation layer currently reports: "at least one SSH public key is required"
		assert.Contains(t, strings.ToLower(err.Error()), "ssh public key")
		assert.Contains(t, strings.ToLower(err.Error()), "required")
	})

	t.Run("invalid config - malformed SSH key", func(t *testing.T) {
		tempDir := t.TempDir()
		developerDir := filepath.Join(tempDir, "alice")
		require.NoError(t, os.MkdirAll(developerDir, 0o755))

		configPath := filepath.Join(developerDir, "devenv-config.yaml")
		configYAML := `name: alice
sshPublicKey: "ssh-rsa not-base64 user"
`
		require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0o644))

		_, err := LoadDeveloperConfig(tempDir, "alice")
		require.Error(t, err)
		// Error may flow from ssh_keys validator or its wrapper message
		assert.Contains(t, strings.ToLower(err.Error()), "ssh")
		assert.Contains(t, strings.ToLower(err.Error()), "invalid")
	})

	t.Run("invalid config - bad CPU value", func(t *testing.T) {
		tempDir := t.TempDir()
		developerDir := filepath.Join(tempDir, "alice")
		require.NoError(t, os.MkdirAll(developerDir, 0o755))

		configPath := filepath.Join(developerDir, "devenv-config.yaml")
		configYAML := `name: alice
sshPublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI alice@example.com"
resources:
  cpu: "abc"   # invalid
  memory: "8Gi"
`
		require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0o644))

		_, err := LoadDeveloperConfig(tempDir, "alice")
		require.Error(t, err)
		// Depending on where it fails, message may indicate cpu invalid/parse/validation
		assert.Contains(t, strings.ToLower(err.Error()), "cpu")
	})
}

func TestLoadDeveloperConfigWithGlobalDefaults(t *testing.T) {
	t.Run("complete integration - global and user config", func(t *testing.T) {
		tempDir := t.TempDir()

		// Global config (provides defaults + base packages + base SSH key)
		globalConfigYAML := `image: "global:latest"
installHomebrew: true
clearLocalPackages: true
packages:
  apt: ["curl", "git"]
  python: ["requests"]
resources:
  cpu: 4
  memory: "16Gi"
sshPublicKey: "ssh-rsa AAAAB3NzaC1yc2E admin@company.com"
`
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "devenv.yaml"), []byte(globalConfigYAML), 0o644))

		// User config (overrides and additive lists)
		developerDir := filepath.Join(tempDir, "alice")
		require.NoError(t, os.MkdirAll(developerDir, 0o755))

		userConfigYAML := `name: alice
sshPublicKey: "ssh-rsa AAAAB3NzaC1yc2E alice@example.com"
installHomebrew: false
packages:
  apt: ["vim"]
  python: ["pandas"]
git:
  name: "Alice Smith"
  email: "alice@example.com"
`
		require.NoError(t, os.WriteFile(filepath.Join(developerDir, "devenv-config.yaml"), []byte(userConfigYAML), 0o644))

		// Load global (should normalize canonical CPU/Mem)
		globalCfg, err := LoadGlobalConfig(tempDir)
		require.NoError(t, err)

		// Load user with global defaults as base (merge + normalize + validate)
		cfg, err := LoadDeveloperConfigWithBaseConfig(tempDir, "alice", globalCfg)
		require.NoError(t, err)

		// User-specific fields
		assert.Equal(t, "alice", cfg.Name)
		assert.Equal(t, "Alice Smith", cfg.Git.Name)
		assert.Equal(t, "alice@example.com", cfg.Git.Email)

		// Overrides and inherited values
		assert.Equal(t, "global:latest", cfg.Image) // user didn't specify; inherited from global
		assert.False(t, cfg.InstallHomebrew)        // user overrides global=true → false
		assert.True(t, cfg.ClearLocalPackages)      // inherited from global

		// Canonical resource units (CPU millicores, Memory MiB)
		assert.Equal(t, int64(4000), cfg.Resources.CPU)       // 4 cores → 4000m
		assert.Equal(t, int64(16*1024), cfg.Resources.Memory) // 16Gi → 16384Mi
		assert.Equal(t, "4000m", cfg.CPU())                   // formatted getter
		assert.Equal(t, "16Gi", cfg.Memory())                 // formatted getter

		// Additive list merging (global first, then user)
		assert.Equal(t, []string{"curl", "git", "vim"}, cfg.Packages.APT)
		assert.Equal(t, []string{"requests", "pandas"}, cfg.Packages.Python)

		// SSH keys merge (global + user). Order depends on your mergeListFields; this expects global first.
		keys, err := cfg.GetSSHKeys()
		require.NoError(t, err)
		assert.Equal(t,
			[]string{
				"ssh-rsa AAAAB3NzaC1yc2E admin@company.com",
				"ssh-rsa AAAAB3NzaC1yc2E alice@example.com",
			},
			keys,
		)

		// Developer directory set
		assert.Equal(t, developerDir, cfg.DeveloperDir)
	})

	t.Run("user config with no global config", func(t *testing.T) {
		tempDir := t.TempDir()

		// Only user config (no devenv.yaml)
		developerDir := filepath.Join(tempDir, "alice")
		require.NoError(t, os.MkdirAll(developerDir, 0o755))

		userConfigYAML := `name: alice
sshPublicKey: "ssh-rsa AAAAB3NzaC1yc2E alice@example.com"
installHomebrew: false
`
		require.NoError(t, os.WriteFile(filepath.Join(developerDir, "devenv-config.yaml"), []byte(userConfigYAML), 0o644))

		// Global = system defaults (no file present)
		globalCfg, err := LoadGlobalConfig(tempDir)
		require.NoError(t, err)

		cfg, err := LoadDeveloperConfigWithBaseConfig(tempDir, "alice", globalCfg)
		require.NoError(t, err)

		// Defaults + user overrides
		assert.Equal(t, "alice", cfg.Name)
		assert.Equal(t, "ubuntu:22.04", cfg.Image)          // system default
		assert.False(t, cfg.InstallHomebrew)                // user override
		assert.False(t, cfg.ClearLocalPackages)             // system default
		assert.Equal(t, "/opt/venv/bin", cfg.PythonBinPath) // system default

		// Canonical resource defaults and formatted getters
		assert.Equal(t, int64(2000), cfg.Resources.CPU)      // default 2 cores
		assert.Equal(t, int64(8*1024), cfg.Resources.Memory) // default 8Gi
		assert.Equal(t, "2000m", cfg.CPU())
		assert.Equal(t, "8Gi", cfg.Memory())
	})
}

func TestMergeListFields(t *testing.T) {
	t.Run("merge packages", func(t *testing.T) {
		globalConfig := &BaseConfig{
			Packages: PackageConfig{
				APT:    []string{"curl", "git"},
				Python: []string{"requests"},
			},
		}

		userConfig := &DevEnvConfig{
			BaseConfig: BaseConfig{
				Packages: PackageConfig{
					APT:    []string{"vim", "curl"}, // "curl" is duplicate
					Python: []string{"pandas"},
				},
			},
		}

		userConfig.mergeListFields(globalConfig)

		expectedAPT := []string{"curl", "git", "vim"} // Deduplication
		expectedPython := []string{"requests", "pandas"}

		assert.Equal(t, expectedAPT, userConfig.Packages.APT)
		assert.Equal(t, expectedPython, userConfig.Packages.Python)
	})

	t.Run("merge volumes", func(t *testing.T) {
		globalConfig := &BaseConfig{
			Volumes: []VolumeMount{
				{Name: "data", LocalPath: "/global/data", ContainerPath: "/data"},
				{Name: "logs", LocalPath: "/global/logs", ContainerPath: "/logs"},
			},
		}

		userConfig := &DevEnvConfig{
			BaseConfig: BaseConfig{
				Volumes: []VolumeMount{
					{Name: "data", LocalPath: "/user/data", ContainerPath: "/data"}, // Same name - user overrides
					{Name: "cache", LocalPath: "/user/cache", ContainerPath: "/cache"},
				},
			},
		}

		userConfig.mergeListFields(globalConfig)

		expected := []VolumeMount{
			{Name: "logs", LocalPath: "/global/logs", ContainerPath: "/logs"},  // From global (not overridden)
			{Name: "data", LocalPath: "/user/data", ContainerPath: "/data"},    // User overrides global
			{Name: "cache", LocalPath: "/user/cache", ContainerPath: "/cache"}, // User only
		}

		assert.ElementsMatch(t, expected, userConfig.Volumes)
	})

	t.Run("merge SSH keys", func(t *testing.T) {
		globalConfig := &BaseConfig{
			SSHPublicKey: []string{"ssh-rsa AAAAB3... admin@company.com"},
		}

		userConfig := &DevEnvConfig{
			BaseConfig: BaseConfig{
				SSHPublicKey: []string{"ssh-rsa AAAAB3... alice@example.com"},
			},
		}

		userConfig.mergeListFields(globalConfig)

		sshKeys, err := userConfig.GetSSHKeys()
		require.NoError(t, err)

		expected := []string{
			"ssh-rsa AAAAB3... admin@company.com", // Global
			"ssh-rsa AAAAB3... alice@example.com", // User
		}
		assert.Equal(t, expected, sshKeys)
	})
}

func TestValidateConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := &DevEnvConfig{
			Name: "alice",
			BaseConfig: BaseConfig{
				SSHPublicKey: "ssh-rsa AAAAB3NzaC1yc2E alice@example.com",
			},
		}

		err := ValidateDevEnvConfig(config)
		assert.NoError(t, err)
	})

	t.Run("missing name", func(t *testing.T) {
		config := &DevEnvConfig{
			BaseConfig: BaseConfig{
				SSHPublicKey: "ssh-rsa AAAAB3NzaC1yc2E... alice@example.com",
			},
		}

		err := ValidateDevEnvConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "'Name' is required")
	})

	t.Run("missing SSH public key", func(t *testing.T) {
		config := &DevEnvConfig{
			Name: "alice",
		}

		err := ValidateDevEnvConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SSH public key is required")
	})

	t.Run("invalid SSH key format", func(t *testing.T) {
		config := &DevEnvConfig{
			Name: "alice",
			BaseConfig: BaseConfig{
				SSHPublicKey: 123, // Invalid type
			},
		}

		err := ValidateDevEnvConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid SSH key format")
	})

	t.Run("empty SSH key", func(t *testing.T) {
		config := &DevEnvConfig{
			Name: "alice",
			BaseConfig: BaseConfig{
				SSHPublicKey: "",
			},
		}

		err := ValidateDevEnvConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid SSH key format")
	})
}

// Test utility functions
func TestMergeStringSlices(t *testing.T) {
	tests := []struct {
		name     string
		global   []string
		user     []string
		expected []string
	}{
		{
			name:     "no duplicates",
			global:   []string{"a", "b"},
			user:     []string{"c", "d"},
			expected: []string{"a", "b", "c", "d"},
		},
		{
			name:     "with duplicates",
			global:   []string{"a", "b"},
			user:     []string{"b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty global",
			global:   []string{},
			user:     []string{"a", "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "empty user",
			global:   []string{"a", "b"},
			user:     []string{},
			expected: []string{"a", "b"},
		},
		{
			name:     "both empty",
			global:   []string{},
			user:     []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeStringSlices(tt.global, tt.user)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeVolumes(t *testing.T) {
	global := []VolumeMount{
		{Name: "data", LocalPath: "/global/data", ContainerPath: "/data"},
		{Name: "logs", LocalPath: "/global/logs", ContainerPath: "/logs"},
	}
	user := []VolumeMount{
		{Name: "data", LocalPath: "/user/data", ContainerPath: "/data"}, // Override
		{Name: "cache", LocalPath: "/user/cache", ContainerPath: "/cache"},
	}

	result := mergeVolumes(global, user)

	// User "data" should override global "data", "logs" should remain, "cache" should be added
	expected := []VolumeMount{
		{Name: "logs", LocalPath: "/global/logs", ContainerPath: "/logs"},
		{Name: "data", LocalPath: "/user/data", ContainerPath: "/data"},
		{Name: "cache", LocalPath: "/user/cache", ContainerPath: "/cache"},
	}

	assert.ElementsMatch(t, expected, result)
}

func TestNormalizeSSHKeys(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		expected    []string
		expectError bool
	}{
		{
			name:        "single string",
			input:       "ssh-rsa AAAAB3... user@host",
			expected:    []string{"ssh-rsa AAAAB3... user@host"},
			expectError: false,
		},
		{
			name:        "string array",
			input:       []string{"key1", "key2"},
			expected:    []string{"key1", "key2"},
			expectError: false,
		},
		{
			name:        "interface array",
			input:       []interface{}{"key1", "key2"},
			expected:    []string{"key1", "key2"},
			expectError: false,
		},
		{
			name:        "nil input",
			input:       nil,
			expected:    []string{},
			expectError: false,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "empty array",
			input:       []string{},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "invalid type",
			input:       123,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeSSHKeys(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func Test_toMi_NormalizesInputs(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64 // Mi
		ok   bool
	}{
		// --- Binary units (powers of 2) ---
		{"Gi exact", "16Gi", 16 * 1024, true},
		{"Mi exact", "512Mi", 512, true},
		{"Ki to Mi", "1024Ki", 1, true},
		{"Gi decimal", "2.5Gi", 2560, true},
		{"trim + casefold", " 2gi ", 2 * 1024, true}, // if you allow case-insensitive units

		// --- Decimal SI (powers of 10) ---
		{"500M", "500M", 477, true}, // 500e6 / 2^20 ≈ 476.84 → round to 477
		{"1G", "1G", 954, true},

		// --- Bare numeric strings (policy: treat as Gi) ---
		{"bare int string", "15", 15 * 1024, true},
		{"bare float string", "1.5", 1536, true},

		// --- Non-string numerics (policy: Gi) ---
		{"int means Gi", 2, 2 * 1024, true},
		{"float means Gi", 1.5, 1536, true},
		{"uint means Gi", uint(4), 4 * 1024, true},

		// --- Zero/negative are invalid ---
		{"zero Gi invalid", "0Gi", 0, false},
		{"zero Mi invalid", "0Mi", 0, false},
		{"bare zero invalid", "0", 0, false},
		{"zero int invalid", 0, 0, false},
		{"negative Gi invalid", "-1Gi", 0, false},
		{"negative int invalid", -1, 0, false},

		// --- Invalid shapes ---
		{"invalid unit", "12GB", 0, false}, // unsupported 'GB'
		{"nonnumeric", "abc", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toMi(tc.in)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeCPU_ToMillicores(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want int64
		ok   bool
	}{
		{"int", 3, 3000, true},
		{"float", 1.25, 1250, true},
		{"string int", "4", 4000, true},
		{"string decimal", "2.5", 2500, true},
		{"millicores", "500m", 500, true},

		{"zero string invalid", "0", 0, false},
		{"zero millicores invalid", "0m", 0, false},
		{"zero int invalid", 0, 0, false},
		{"negative int invalid", -1, 0, false},
		{"negative string decimal invalid", "-2.5", 0, false},
		{"negative millicores invalid", "-250m", 0, false},

		{"spaces", " 3 ", 3000, true},
		{"leading zeros", "0003", 3000, true},

		{"invalid", "abc", 0, false},
		{"invalid suffix", "5M", 0, false}, // only lowercase 'm' for millicores
		{"nil", nil, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toMillicores(tc.raw)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
