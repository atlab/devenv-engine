package config

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewBaseConfigWithDefaults verifies that the factory defaults produce a
// ready-to-use config with canonical resource units and non-nil empty slices.
// This test documents the contract of NewBaseConfigWithDefaults so future changes
// (e.g., image tag, default resources, or slice initialization) don’t regress silently.
func TestNewBaseConfigWithDefaults(t *testing.T) {
	cfg := NewBaseConfigWithDefaults()

	// --- Basic defaults (scalar fields) ---
	assert.Equal(t, "ubuntu:22.04", cfg.Image)
	assert.Equal(t, 1000, cfg.UID)
	assert.Equal(t, "/opt/venv/bin", cfg.PythonBinPath)

	// --- Container setup toggles ---
	assert.True(t, cfg.InstallHomebrew)
	assert.False(t, cfg.ClearLocalPackages)
	assert.False(t, cfg.ClearVSCodeCache)

	// --- Resource defaults (canonical units) ---
	// Canonical CPU is millicores; default should be 2 cores = 2000m.
	assert.Equal(t, int64(2000), cfg.Resources.CPU)
	// Canonical Memory is Mi; default should be 8 Gi = 8192 Mi.
	assert.Equal(t, int64(8192), cfg.Resources.Memory)
	assert.Equal(t, "20Gi", cfg.Resources.Storage)
	assert.Equal(t, 0, cfg.Resources.GPU)

	// Also assert getters render the canonical values as expected (future-proofing).
	assert.Equal(t, "2000m", (&DevEnvConfig{BaseConfig: cfg}).CPU())
	assert.Equal(t, "8Gi", (&DevEnvConfig{BaseConfig: cfg}).Memory())

	// Raw fields should be empty in defaults (we set canonical values directly).
	assert.Nil(t, cfg.Resources.CPURaw)
	assert.Nil(t, cfg.Resources.MemoryRaw)

	// --- Empty-but-non-nil slices (ergonomic contract) ---
	// Callers can append without nil checks; order is preserved.
	assert.NotNil(t, cfg.Packages.Python)
	assert.NotNil(t, cfg.Packages.APT)
	assert.NotNil(t, cfg.Volumes)
	assert.Len(t, cfg.Packages.Python, 0)
	assert.Len(t, cfg.Packages.APT, 0)
	assert.Len(t, cfg.Volumes, 0)

	// Appending to default-initialized slices should not panic.
	assert.NotPanics(t, func() { cfg.Packages.Python = append(cfg.Packages.Python, "numpy") })
	assert.NotPanics(t, func() { cfg.Packages.APT = append(cfg.Packages.APT, "curl") })
	assert.NotPanics(t, func() { cfg.Volumes = append(cfg.Volumes, VolumeMount{Name: "work"}) })
}

// TestBaseConfig_GetSSHKeys verifies that SSH keys are normalized from flexible
// YAML shapes (string, []string, []any) into a clean []string, with trimming,
// order preserved, and clear failures for invalid/empty inputs.
func TestBaseConfig_GetSSHKeys(t *testing.T) {
	tests := []struct {
		name        string
		sshKeyField any
		expected    []string
		expectError bool
	}{
		{
			name:        "single string key",
			sshKeyField: "ssh-rsa AAAAB3NzaC1yc2E... user@host",
			expected:    []string{"ssh-rsa AAAAB3NzaC1yc2E... user@host"},
			expectError: false,
		},
		{
			name:        "single string trimmed",
			sshKeyField: "   ssh-ed25519 AAAAC3... user   ",
			expected:    []string{"ssh-ed25519 AAAAC3... user"},
			expectError: false,
		},
		{
			name:        "multiple string keys preserves order",
			sshKeyField: []string{"ssh-rsa AAAAB3... user1", "ssh-ed25519 AAAAC3... user2"},
			expected:    []string{"ssh-rsa AAAAB3... user1", "ssh-ed25519 AAAAC3... user2"},
			expectError: false,
		},
		{
			name:        "interface slice from YAML",
			sshKeyField: []any{"ssh-rsa AAAAB3... user1", "ssh-ed25519 AAAAC3... user2"},
			expected:    []string{"ssh-rsa AAAAB3... user1", "ssh-ed25519 AAAAC3... user2"},
			expectError: false,
		},
		{
			name:        "nil field yields empty slice (safe default)",
			sshKeyField: nil,
			expected:    []string{},
			expectError: false,
		},
		{
			name:        "empty string key is invalid",
			sshKeyField: "",
			expected:    []string{},
			expectError: true,
		},
		// TODO: Return to this
		{
			name:        "empty slice is invalid",
			sshKeyField: []string{},
			expected:    nil, // ignored when expectError=true
			expectError: true,
		},
		{
			name:        "slice containing blank entry is invalid",
			sshKeyField: []string{"ssh-rsa AAAAB3... user", "   "},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "mixed-type slice is invalid",
			sshKeyField: []any{"ssh-rsa AAAAB3... user", 42},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "invalid type (int) is invalid",
			sshKeyField: 123,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &BaseConfig{SSHPublicKey: tt.sshKeyField}
			got, err := cfg.GetSSHKeys()

			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestDevEnvConfig_GetUserID verifies that UID is formatted as a string with no
// implicit defaulting or validation at this layer (pure accessor behavior).
func TestDevEnvConfig_GetUserID(t *testing.T) {
	tests := []struct {
		name     string
		uid      int
		expected string
	}{
		{name: "custom UID", uid: 2000, expected: "2000"},
		{name: "zero UID returns '0'", uid: 0, expected: "0"},
		{name: "negative UID is formatted as-is", uid: -7, expected: "-7"}, // validation happens elsewhere
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &DevEnvConfig{BaseConfig: BaseConfig{UID: tt.uid}}
			assert.Equal(t, tt.expected, cfg.GetUserID())
		})
	}
}

// TestDevEnvConfig_CPU_Format verifies that CPU() is a pure formatter over the
// canonical millicores field (Resources.CPU). It does not parse raw inputs or
// apply defaults—those happen during normalization.
func TestDevEnvConfig_CPU_Format(t *testing.T) {
	tests := []struct {
		name  string
		milli int64
		want  string
	}{
		{name: "zero -> 0", milli: 0, want: "0"},
		{name: "positive -> Xm", milli: 2500, want: "2500m"},
		{name: "large -> Xm", milli: 123456, want: "123456m"},
		{name: "negative clamps", milli: -1, want: "0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{
				BaseConfig: BaseConfig{
					Resources: ResourceConfig{CPU: tc.milli},
				},
			}
			assert.Equal(t, tc.want, cfg.CPU())
		})
	}
}

// CPU() must format canonical Resources.CPU and ignore CPURaw if canonical isn't set.
func TestDevEnvConfig_CPU_IgnoresRawWithoutNormalization(t *testing.T) {
	// Raw provided, canonical zero => CPU() should not look at CPURaw.
	cfg := &DevEnvConfig{
		BaseConfig: BaseConfig{
			Resources: ResourceConfig{CPURaw: "2.5", CPU: 0},
		},
	}
	require.Equal(t, "0", cfg.CPU())

	// Canonical set => CPU() formats it regardless of CPURaw.
	cfg2 := &DevEnvConfig{
		BaseConfig: BaseConfig{
			Resources: ResourceConfig{CPURaw: "bad", CPU: 2500},
		},
	}
	require.Equal(t, "2500m", cfg2.CPU())
}

// TestDevEnvConfig_Memory_Format verifies that Memory() is a pure formatter over the
// canonical Mi field (Resources.Memory). It does not parse raw inputs; normalization
// must have already populated Memory in MiB. Exact Gi multiples render as "Gi", otherwise "Mi".
func TestDevEnvConfig_Memory_Format(t *testing.T) {
	tests := []struct {
		name  string
		memMi int64
		want  string
	}{
		{name: "zero -> empty", memMi: 0, want: ""},
		{name: "exact Gi prints Gi", memMi: 16 * 1024, want: "16Gi"},
		{name: "non-Gi prints Mi", memMi: 1536, want: "1536Mi"}, // 1.5 Gi
		{name: "small Mi prints Mi", memMi: 477, want: "477Mi"},
		{name: "negative -> empty", memMi: -1, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{
				BaseConfig: BaseConfig{
					Resources: ResourceConfig{Memory: tc.memMi},
				},
			}
			assert.Equal(t, tc.want, cfg.Memory())
		})
	}
}

// Memory() must format canonical Resources.Memory and ignore MemoryRaw if canonical isn't set.
func TestDevEnvConfig_Memory_IgnoresRawWithoutNormalization(t *testing.T) {
	cfg := &DevEnvConfig{
		BaseConfig: BaseConfig{
			Resources: ResourceConfig{MemoryRaw: "1.5Gi", Memory: 0},
		},
	}
	require.Equal(t, "", cfg.Memory())

	cfg2 := &DevEnvConfig{
		BaseConfig: BaseConfig{
			Resources: ResourceConfig{MemoryRaw: "bad", Memory: 1536},
		},
	}
	require.Equal(t, "1536Mi", cfg2.Memory())
}

// TestDevEnvConfig_Memory_FormatFromCanonical verifies that Memory() is a pure
// formatter over the canonical Mi field (Resources.Memory). Exact Gi multiples
// render with "Gi"; all other positive values render with "Mi"; non-positive
// values render as an empty string.
func TestDevEnvConfig_Memory_FormatFromCanonical(t *testing.T) {
	tests := []struct {
		name   string
		memory int64 // canonical Mi
		want   string
	}{
		{name: "zero -> empty", memory: 0, want: ""},
		{name: "negative -> empty", memory: -1, want: ""},
		{name: "exact Gi", memory: 16 * 1024, want: "16Gi"},
		{name: "non-Gi (1.5Gi)", memory: 1536, want: "1536Mi"},
		{name: "non-Gi (small Mi)", memory: 477, want: "477Mi"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{
				BaseConfig: BaseConfig{
					Resources: ResourceConfig{Memory: tc.memory},
				},
			}
			got := cfg.Memory()
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestDevEnvConfig_GPU documents the contract of GPU(): it returns a non-negative
// GPU count by clamping negatives to zero, and defaults to zero when unset.
func TestDevEnvConfig_GPU(t *testing.T) {
	tests := []struct {
		name     string
		gpu      int
		expected int
	}{
		{name: "positive", gpu: 2, expected: 2},
		{name: "zero", gpu: 0, expected: 0},
		{name: "negative clamped to zero", gpu: -1, expected: 0},
		{name: "large value preserved", gpu: 8, expected: 8},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{
				BaseConfig: BaseConfig{
					Resources: ResourceConfig{GPU: tc.gpu},
				},
			}
			got := cfg.GPU()
			assert.Equal(t, tc.expected, got)
		})
	}

	t.Run("unset defaults to zero", func(t *testing.T) {
		// Do not set GPU at all; rely on zero-values.
		cfg := &DevEnvConfig{} // BaseConfig.Resources.GPU == 0 by default
		assert.Equal(t, 0, cfg.GPU())
	})
}

func TestDevEnvConfig_NodePort(t *testing.T) {
	tests := []struct {
		name     string
		sshPort  int
		expected int
	}{
		{name: "typical value", sshPort: 30022, expected: 30022},
		{name: "lower bound", sshPort: 30000, expected: 30000},
		{name: "upper bound", sshPort: 32767, expected: 32767},

		// Out-of-range values still pass through here.
		// Range enforcement is tested in validation (ports.go) tests.
		{name: "below range", sshPort: 29999, expected: 29999},
		{name: "above range", sshPort: 32768, expected: 32768},

		// Degenerate cases
		{name: "zero", sshPort: 0, expected: 0},
		{name: "negative", sshPort: -1, expected: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{SSHPort: tc.sshPort}
			got := cfg.NodePort()
			assert.Equal(t, tc.expected, got)
		})
	}
}

// TestDevEnvConfig_VolumeMounts verifies that VolumeMounts() returns a stable,
// template-friendly view of the configured mounts:
//   - order is preserved
//   - empty vs. nil are preserved
//   - it returns a defensive copy (mutations to the returned slice do not affect config)
func TestDevEnvConfig_VolumeMounts(t *testing.T) {
	tests := []struct {
		name     string
		vols     []VolumeMount
		expected []VolumeMount
	}{
		{
			name: "multiple entries preserve order",
			vols: []VolumeMount{
				{Name: "data", LocalPath: "/local/data", ContainerPath: "/data"},
				{Name: "logs", LocalPath: "/local/logs", ContainerPath: "/logs"},
			},
			expected: []VolumeMount{
				{Name: "data", LocalPath: "/local/data", ContainerPath: "/data"},
				{Name: "logs", LocalPath: "/local/logs", ContainerPath: "/logs"},
			},
		},
		{
			name:     "empty slice returns empty (non-nil)",
			vols:     []VolumeMount{},
			expected: []VolumeMount{},
		},
		{
			name:     "nil slice stays nil",
			vols:     nil,
			expected: nil,
		},
		{
			name:     "single entry",
			vols:     []VolumeMount{{Name: "workspace", LocalPath: "/src", ContainerPath: "/work"}},
			expected: []VolumeMount{{Name: "workspace", LocalPath: "/src", ContainerPath: "/work"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{BaseConfig: BaseConfig{Volumes: tc.vols}}

			// Basic equality and empty/nil shape
			got := cfg.VolumeMounts()
			assert.Equal(t, tc.expected, got)

			// Immutability: mutate the returned slice; original must not change.
			before := cfg.BaseConfig.Volumes
			if got != nil {
				// append to returned slice
				got = append(got, VolumeMount{Name: "tmp", LocalPath: "/tmp", ContainerPath: "/tmp"})
				// modify element content
				if len(got) > 0 {
					got[0].Name = "mutated"
				}
			}
			// Config’s stored volumes must remain identical to the original input
			assert.Equal(t, before, cfg.BaseConfig.Volumes)
		})
	}
}

// TestDevEnvConfig_GetSSHKeysSlice verifies the template-friendly accessor:
// - It normalizes flexible shapes into []string (string, []string, []any of string).
// - On any normalization error, it returns an empty slice (never nil).
// - It returns a defensive copy: mutating the result doesn't affect subsequent calls.
func TestDevEnvConfig_GetSSHKeysSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{
			name:     "valid slice preserves order",
			input:    []string{"ssh-rsa AAAA user1", "ssh-ed25519 BBBB user2"},
			expected: []string{"ssh-rsa AAAA user1", "ssh-ed25519 BBBB user2"},
		},
		{
			name:     "single string normalizes to slice",
			input:    "ssh-ed25519 BBBB user",
			expected: []string{"ssh-ed25519 BBBB user"},
		},
		{
			name:     "single string trimmed",
			input:    "   ssh-rsa AAAA user   ",
			expected: []string{"ssh-rsa AAAA user"},
		},
		{
			name:     "interface slice (YAML) preserves order",
			input:    []any{"ssh-rsa AAAA user1", "ssh-ed25519 BBBB user2"},
			expected: []string{"ssh-rsa AAAA user1", "ssh-ed25519 BBBB user2"},
		},
		{
			name:     "nil yields empty slice",
			input:    nil,
			expected: []string{},
		},
		{
			// empty slice is an error in normalizeSSHKeys; accessor suppresses to empty
			name:     "empty slice yields empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "invalid type suppressed to empty",
			input:    123, // wrong type
			expected: []string{},
		},
		{
			name:     "mixed-type interface slice suppressed to empty",
			input:    []any{"ssh-rsa AAAA user", 42},
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{BaseConfig: BaseConfig{SSHPublicKey: tc.input}}

			got1 := cfg.GetSSHKeysSlice()
			assert.Equal(t, tc.expected, got1)
			assert.NotNil(t, got1) // accessor always returns a slice, never nil

			// Immutability / defensive copy: mutate the returned slice, call again; result should be unchanged.
			if len(got1) > 0 {
				got1[0] = "MUTATED"
			}
			got2 := cfg.GetSSHKeysSlice()
			assert.Equal(t, tc.expected, got2) // must not reflect caller mutations
		})
	}
}

// TestDevEnvConfig_GetDeveloperDir verifies that GetDeveloperDir() is a pure accessor:
// it returns exactly what is stored (no cleaning, normalization, or validation).
func TestDevEnvConfig_GetDeveloperDir(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "absolute path", path: "/path/to/developers/alice", want: "/path/to/developers/alice"},
		{name: "relative path", path: "developers/alice", want: "developers/alice"},
		{name: "empty string", path: "", want: ""},
		{name: "trailing slash preserved", path: "/devs/alice/", want: "/devs/alice/"},
		{name: "unicode / spaces preserved", path: "/devs/álïçë projects", want: "/devs/álïçë projects"},

		// No normalization: leading dot or parent segments are returned as-is.
		{name: "dot-segment retained", path: "./devs/alice", want: "./devs/alice"},
		{name: "parent-segment retained", path: "../devs/alice", want: "../devs/alice"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{DeveloperDir: tc.path}
			got := cfg.GetDeveloperDir()
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("unset (zero-value struct) -> empty", func(t *testing.T) {
		var cfg DevEnvConfig // DeveloperDir is zero value (empty string)
		assert.Equal(t, "", cfg.GetDeveloperDir())
	})
}

// TestDevEnvConfig_CPURequest_AliasOfCPU documents that CPURequest() returns
// exactly what CPU() returns for canonical millicores. This test does not
// exercise parsing/normalization; it only covers the formatting layer.
func TestDevEnvConfig_CPURequest_AliasOfCPU(t *testing.T) {
	tests := []struct {
		name  string
		milli int64 // canonical millicores
	}{
		{name: "zero", milli: 0},
		{name: "positive", milli: 2500},
		{name: "large", milli: 123456},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{
				BaseConfig: BaseConfig{
					Resources: ResourceConfig{CPU: tc.milli},
				},
			}
			gotCPU := cfg.CPU()
			gotReq := cfg.CPURequest()
			assert.Equal(t, gotCPU, gotReq)
		})
	}

	// Guard: CPURaw alone (without normalization) has no effect on getters.
	t.Run("raw value ignored without normalization", func(t *testing.T) {
		cfg := &DevEnvConfig{
			BaseConfig: BaseConfig{
				Resources: ResourceConfig{CPURaw: "2.5", CPU: 0},
			},
		}
		assert.Equal(t, "0", cfg.CPU())
		assert.Equal(t, cfg.CPU(), cfg.CPURequest())
	})
}

// TestDevEnvConfig_MemoryRequest_AliasOfMemory documents that MemoryRequest()
// returns exactly what Memory() returns for canonical mebibytes (Mi). This
// exercises the formatting layer only; parsing/normalization is tested elsewhere.
func TestDevEnvConfig_MemoryRequest_AliasOfMemory(t *testing.T) {
	tests := []struct {
		name  string
		memMi int64 // canonical Mi
	}{
		{name: "zero -> empty", memMi: 0},
		{name: "exact Gi formats Gi", memMi: 16 * 1024}, // "16Gi"
		{name: "non-Gi (1.5Gi) -> Mi", memMi: 1536},     // "1536Mi"
		{name: "non-Gi (small Mi)", memMi: 477},         // "477Mi"
		// Under strict normalization, canonical should not be negative; this is defensive.
		{name: "negative -> empty (defensive)", memMi: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &DevEnvConfig{
				BaseConfig: BaseConfig{
					Resources: ResourceConfig{Memory: tc.memMi},
				},
			}
			gotMem := cfg.Memory()
			gotReq := cfg.MemoryRequest()
			assert.Equal(t, gotMem, gotReq)
		})
	}

	// Guard: MemoryRaw alone (without normalization) has no effect on getters.
	t.Run("raw value ignored without normalization", func(t *testing.T) {
		cfg := &DevEnvConfig{
			BaseConfig: BaseConfig{
				Resources: ResourceConfig{MemoryRaw: "1.5Gi", Memory: 0},
			},
		}
		assert.Equal(t, "", cfg.Memory())
		assert.Equal(t, cfg.Memory(), cfg.MemoryRequest())
	})
}

// TestDevEnvConfig_Embedding verifies Go struct embedding behavior for this API:
// - BaseConfig fields are promoted and readable directly on DevEnvConfig.
// - Methods defined on BaseConfig are promoted to DevEnvConfig.
// - DevEnvConfig-owned fields remain accessible.
// - The embedded BaseConfig is not copied (promoted fields reflect the same storage).
func TestDevEnvConfig_Embedding(t *testing.T) {
	cfg := &DevEnvConfig{
		BaseConfig: BaseConfig{
			Image:              "custom:latest",
			InstallHomebrew:    false,
			ClearLocalPackages: true,
			PythonBinPath:      "/custom/python/bin",
			UID:                2000,
		},
		Name: "alice",
		Git: GitConfig{
			Name:  "Alice Smith",
			Email: "alice@example.com",
		},
	}

	// 1) Field promotion: embedded fields readable directly
	assert.Equal(t, "custom:latest", cfg.Image)
	assert.False(t, cfg.InstallHomebrew)
	assert.True(t, cfg.ClearLocalPackages)
	assert.Equal(t, "/custom/python/bin", cfg.PythonBinPath)
	assert.Equal(t, 2000, cfg.UID)

	// 2) Method promotion: methods on BaseConfig are callable on DevEnvConfig
	assert.Equal(t, "2000", cfg.GetUserID())

	// 3) Owned fields: user-specific fields
	assert.Equal(t, "alice", cfg.Name)
	assert.Equal(t, "Alice Smith", cfg.Git.Name)
	assert.Equal(t, "alice@example.com", cfg.Git.Email)

	// 4) Same storage: mutate via promoted field and check embedded struct mirrors it
	cfg.Image = "changed:tag"
	assert.Equal(t, "changed:tag", cfg.BaseConfig.Image)

	// Also mutate via embedded and check promoted view updates
	cfg.BaseConfig.InstallHomebrew = true
	assert.True(t, cfg.InstallHomebrew)
}

// Verifies NewBaseConfigWithDefaults returns canonical resource units:
// CPU in millicores (2000 = 2 cores) and Memory in Mi (8192 = 8Gi).
func TestNewBaseConfigWithDefaults_ExactValues(t *testing.T) {
	cfg := NewBaseConfigWithDefaults()

	// Top-level fields
	require.Equal(t, "ubuntu:22.04", cfg.Image)
	require.Equal(t, 1000, cfg.UID)
	require.True(t, cfg.InstallHomebrew)
	require.False(t, cfg.ClearLocalPackages)
	require.False(t, cfg.ClearVSCodeCache)
	require.Equal(t, "/opt/venv/bin", cfg.PythonBinPath)

	// Resources (canonical)
	require.Equal(t, int64(2000), cfg.Resources.CPU)    // 2 cores
	require.Equal(t, int64(8192), cfg.Resources.Memory) // 8Gi
	require.Equal(t, "20Gi", cfg.Resources.Storage)
	require.Equal(t, 0, cfg.Resources.GPU)

	// Raw fields should be empty when canonical defaults are used
	require.Nil(t, cfg.Resources.CPURaw)
	require.Nil(t, cfg.Resources.MemoryRaw)

	// Packages: non-nil, length 0
	require.NotNil(t, cfg.Packages.Python)
	require.Len(t, cfg.Packages.Python, 0)
	require.NotNil(t, cfg.Packages.APT)
	require.Len(t, cfg.Packages.APT, 0)

	// Volumes: non-nil, length 0
	require.NotNil(t, cfg.Volumes)
	require.Len(t, cfg.Volumes, 0)

	// Optional: also assert formatter getters (future-proof)
	dev := &DevEnvConfig{BaseConfig: cfg}
	require.Equal(t, "2000m", dev.CPU())
	require.Equal(t, "8Gi", dev.Memory())
}

func TestNewBaseConfigWithDefaults_DeterministicAndIndependent(t *testing.T) {
	a := NewBaseConfigWithDefaults()
	b := NewBaseConfigWithDefaults()

	// Deterministic: two fresh values are deeply equal.
	require.Equal(t, a, b)

	// --- Independence for slices: modifying 'a' does not affect 'b' ---

	// Packages.Python
	a.Packages.Python = append(a.Packages.Python, "numpy")
	require.NotEqual(t, a.Packages.Python, b.Packages.Python)

	// Packages.APT
	a.Packages.APT = append(a.Packages.APT, "curl")
	require.NotEqual(t, a.Packages.APT, b.Packages.APT)

	// Volumes: append and also mutate an element to prove deep independence
	a.Volumes = append(a.Volumes, VolumeMount{Name: "data", LocalPath: "/data", ContainerPath: "/mnt/data"})
	require.NotEqual(t, a.Volumes, b.Volumes)

	// Mutate an existing element if present
	if len(a.Volumes) > 0 {
		a.Volumes[0].Name = "mutated"
		require.NotEqual(t, a.Volumes, b.Volumes)
	}

	// --- Scalars diverge independently as well ---
	// CPU is canonical millicores; change a to 4000m (4 cores)
	a.Resources.CPU = 4000
	require.NotEqual(t, a.Resources.CPU, b.Resources.CPU)

	// Memory is canonical Mi; change a to 16384Mi (16Gi)
	a.Resources.Memory = 16 * 1024
	require.NotEqual(t, a.Resources.Memory, b.Resources.Memory)
}

// Command-line flag for updating golden files
// Usage: go test -v ./internal/templates -update-golden
var _ = flag.Bool("update-golden", false, "update golden files")
