package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//
// --- ssh_keys predicate ------------------------------------------------------
//

func TestValidator_SSHKeys(t *testing.T) {
	type S struct {
		Keys any `validate:"ssh_keys"`
	}
	cases := []struct {
		name string
		val  any
		ok   bool
	}{
		// Accept: single string
		{"single string", "ssh-ed25519 AAAAB3NzaC1lZDI1NTE5AAAA test@host", true},
		// Accept: []string
		{"slice of strings", []string{
			"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ test1@h",
			"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY test2@h",
		}, true},
		// Accept: []any strings (YAML pattern)
		{"interface slice of strings", []any{
			"ssh-ed25519 AAAAB3NzaC1lZDI1NTE5AAAA test@h",
			"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ test2@h",
		}, true},

		// Reject: empty string
		{"empty string", "", false},
		// Reject: empty slice
		{"empty slice", []string{}, false},
		// Reject: mixed types
		{"mixed slice", []any{"ssh-ed25519 AAAA u@h", 42}, false},
		// Reject: malformed key
		{"malformed", "ssh-ed25519 NOT_BASE64", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate.Struct(&S{Keys: tc.val})
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

//
// --- k8s_cpu predicate (raw shape) ------------------------------------------
//

func TestValidator_K8SCPU(t *testing.T) {
	type S struct {
		CPU any `validate:"k8s_cpu"`
	}
	cases := []struct {
		name string
		val  any
		ok   bool
	}{
		// Accept strings
		{"plain int string", "2", true},
		{"decimal string", "2.5", true},
		{"millicores string", "500m", true},
		{"trimmed", " 3.0 ", true},
		{"empty string", "", true}, // policy: empty allowed; caller decides default
		{"unlimited", "unlimited", true},

		// Consider disallowing these?
		// Accept numerics
		{"int", 3, true},
		{"float64", 1.25, true},
		{"uint", uint(4), true},

		// Reject negatives / junk
		{"negative string", "-1", false},
		{"negative int", -2, false},
		{"junk", "abc", false},
		{"bad millicores", "12xm", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate.Struct(&S{CPU: tc.val})
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

//
// --- k8s_memory predicate (raw shape) ---------------------------------------
//

func TestValidator_K8SMemory(t *testing.T) {
	type S struct {
		Mem any `validate:"k8s_memory"`
	}
	cases := []struct {
		name string
		val  any
		ok   bool
	}{
		// Accept strings (binary units, decimal SI, bare)
		{"Mi", "512Mi", true},
		{"Gi", "16Gi", true},
		{"Ki", "1024Ki", true},
		{"decimal SI M", "500M", true},
		{"decimal SI G", "1G", true},
		{"bare", "1536", true},
		{"trimmed", " 2.5Gi ", true},
		{"empty string", "", true},
		{"unlimited", "unlimited", true},

		// Consider disallowing these?
		// Accept numerics (treated as Gi by normalizer later)
		{"int", 2, true},
		{"float64", 1.5, true},

		// Reject negatives / junk / unknown unit
		{"negative string", "-1Gi", false},
		{"junk", "abc", false},
		{"unknown unit", "12GB", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate.Struct(&S{Mem: tc.val})
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

//
// --- ValidateDevEnvConfig (post-normalization semantics) --------------------
//

func TestValidateDevEnvConfig_SSHRequirement(t *testing.T) {
	// Missing keys -> error
	cfgMissing := &DevEnvConfig{
		Name: "alice",
		BaseConfig: BaseConfig{
			SSHPublicKey: nil,
		},
	}
	err := ValidateDevEnvConfig(cfgMissing)
	require.Error(t, err)
	lower := strings.ToLower(err.Error())
	assert.Contains(t, lower, "ssh")
	assert.Contains(t, lower, "required")

	// Present and valid -> ok
	cfgOK := &DevEnvConfig{
		Name: "alice",
		BaseConfig: BaseConfig{
			SSHPublicKey: "ssh-ed25519 AAAAB3NzaC1lZDI1NTE5AAAA user@host",
		},
	}
	require.NoError(t, ValidateDevEnvConfig(cfgOK))
}

func TestValidateDevEnvConfig_ResourcesNonNegative(t *testing.T) {
	// CPU negative
	cfgCPU := &DevEnvConfig{
		Name: "alice",
		BaseConfig: BaseConfig{
			Resources: ResourceConfig{
				CPU: -1, // canonical millicores (already normalized)
			},
			SSHPublicKey: "ssh-ed25519 AAAAB3NzaC1lZDI1NTE5AAAA user@h",
		},
	}
	err := ValidateDevEnvConfig(cfgCPU)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "cpu")

	// Memory negative
	cfgMem := &DevEnvConfig{
		Name: "alice",
		BaseConfig: BaseConfig{
			Resources: ResourceConfig{
				Memory: -1, // canonical MiB (already normalized)
			},
			SSHPublicKey: "ssh-ed25519 AAAAB3NzaC1lZDI1NTE5AAAA user@h",
		},
	}
	err = ValidateDevEnvConfig(cfgMem)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "memory")

	// GPU negative
	cfgGPU := &DevEnvConfig{
		Name: "alice",
		BaseConfig: BaseConfig{
			Resources: ResourceConfig{
				GPU: -1,
			},
			SSHPublicKey: "ssh-ed25519 AAAAB3NzaC1lZDI1NTE5AAAA user@h",
		},
	}
	err = ValidateDevEnvConfig(cfgGPU)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "gpu")

	// All non-negative -> ok
	cfgOK := &DevEnvConfig{
		Name: "alice",
		BaseConfig: BaseConfig{
			Resources: ResourceConfig{
				CPU:    2500,
				Memory: 16 * 1024,
				GPU:    0,
			},
			SSHPublicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ user@h",
		},
	}
	require.NoError(t, ValidateDevEnvConfig(cfgOK))
}

//
// --- ValidateBaseConfig: no tag failures by default -------------------------
//

func TestValidateBaseConfig_Smoke(t *testing.T) {
	// With no validation tags on BaseConfig itself, this should succeed.
	var bc BaseConfig
	require.NoError(t, ValidateBaseConfig(&bc))
}
