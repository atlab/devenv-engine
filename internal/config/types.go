package config

import (
	"fmt"
	"strings"
)

// BaseConfig contains all configuration fields that can be shared between
// global defaults and user-specific configurations. This serves as the foundation
// for both global configuration files and user configurations.
type BaseConfig struct {
	// Resource allocation
	Image     string         `yaml:"image,omitempty" validate:"omitempty,min=1"`
	Resources ResourceConfig `yaml:"resources,omitempty"`
	UID       int            `yaml:"uid,omitempty" validate:"omitempty,min=1000,max=65535"`

	// Package management
	Packages PackageConfig `yaml:"packages,omitempty"`

	// Storage configuration
	Volumes []VolumeMount `yaml:"volumes,omitempty" validate:"dive"`

	// Access configuration
	SSHPublicKey any `yaml:"sshPublicKey,omitempty" validate:"omitempty,ssh_keys"` // Can be string or []string

	// Container setup configuration
	InstallHomebrew    bool   `yaml:"installHomebrew,omitempty"`
	ClearLocalPackages bool   `yaml:"clearLocalPackages,omitempty"`
	ClearVSCodeCache   bool   `yaml:"clearVSCodeCache,omitempty"`
	PythonBinPath      string `yaml:"pythonBinPath,omitempty" validate:"omitempty,min=1,filepath"`
	HostName           string `yaml:"hostName,omitempty" validate:"omitempty,min=1,hostname"`
	EnableAuth         bool   `yaml:"enableAuth,omitempty"`
	AuthURL            string `yaml:"authURL,omitempty" validate:"omitempty,min=1,url"`
	AuthSignIn         string `yaml:"authSignIn,omitempty" validate:"omitempty,min=1,url"`

	// DevENV wide settings
	Namespace       string `yaml:"namespace,omitempty" validate:"omitempty,min=1,max=63,hostname"`
	EnvironmentName string `yaml:"environmentName,omitempty" validate:"omitempty,min=1,max=63,hostname"`
}

// DevEnvConfig represents the complete configuration for a developer environment.
// It embeds BaseConfig for shared fields and adds user-specific fields.
// The struct supports flexible field types where appropriate (e.g., CPU can be
// specified as string, int, or float64) and provides methods for accessing
// values with sensible defaults.
type DevEnvConfig struct {
	BaseConfig `yaml:",inline"` // Embedded - all BaseConfig fields are promoted

	// User-specific fields that don't belong in BaseConfig
	Name         string        `yaml:"name" validate:"required,min=1,max=63,hostname"`
	SSHPort      int           `yaml:"sshPort,omitempty" validate:"omitempty,min=30000,max=32767"`
	HTTPPort     int           `yaml:"httpPort,omitempty" validate:"omitempty,min=1024,max=65535"`
	IsAdmin      bool          `yaml:"isAdmin,omitempty"`
	SkipAuth     bool          `yaml:"skipAuth,omitempty"`
	TargetNodes  []string      `yaml:"targetNodes,omitempty" validate:"dive,hostname"`
	Git          GitConfig     `yaml:"git,omitempty"`
	Refresh      RefreshConfig `yaml:"refresh,omitempty"`
	DeveloperDir string        `yaml:"-"` // Directory where the developer config is located
}

// GitConfig represents Git-related configuration
type GitConfig struct {
	Name  string `yaml:"name,omitempty" validate:"omitempty,min=1,max=100"`
	Email string `yaml:"email,omitempty" validate:"omitempty,email"`
}

// PackageConfig represents package installation configuration
type PackageConfig struct {
	Python []string `yaml:"python,omitempty" validate:"dive,min=1"`
	APT    []string `yaml:"apt,omitempty" validate:"dive,min=1"`
	// Consider adding other package managers such as NPM, Yarn, etc.
}

// ResourceConfig represents resource allocation
type ResourceConfig struct {
	CPU     any    `yaml:"cpu,omitempty" validate:"omitempty,k8s_cpu"`
	Memory  any    `yaml:"memory,omitempty" validate:"omitempty,k8s_memory"`
	Storage string `yaml:"storage,omitempty" validate:"omitempty,k8s_memory"`
	GPU     int    `yaml:"gpu,omitempty" validate:"omitempty,min=0,max=8"` // Number of GPUs requested
}

// VolumeMount represents a volume mount configuration
type VolumeMount struct {
	Name          string `yaml:"name" validate:"required,min=1,max=63,alphanum"`
	LocalPath     string `yaml:"localPath" validate:"required,min=1,filepath"`
	ContainerPath string `yaml:"containerPath" validate:"required,min=1,filepath"`
}

// RefreshConfig represents auto-refresh settings
type RefreshConfig struct {
	Enabled      bool   `yaml:"enabled,omitempty"`
	Schedule     string `yaml:"schedule,omitempty,cron"` // Cron format
	Type         string `yaml:"type,omitempty"`
	PreserveHome bool   `yaml:"preserveHome,omitempty"`
}

// NewBaseConfigWithDefaults creates a BaseConfig instance pre-populated with system defaults
func NewBaseConfigWithDefaults() BaseConfig {
	return BaseConfig{
		Image:              "ubuntu:22.04",
		UID:                1000,
		InstallHomebrew:    true,
		ClearLocalPackages: false,
		ClearVSCodeCache:   false,
		PythonBinPath:      "/opt/venv/bin",
		Resources: ResourceConfig{
			CPU:     2,      // Default CPU
			Memory:  "8Gi",  // Default Memory
			Storage: "20Gi", // Default Storage
			GPU:     0,      // Default GPU
		},
		Packages: PackageConfig{
			Python: []string{}, // Empty slice - no default packages
			APT:    []string{}, // Empty slice - no default packages
		},
		Volumes:         []VolumeMount{}, // Empty slice - no default volumes
		Namespace:       "devenv",        // Default namespace
		EnvironmentName: "development",   // Default environment name
	}
}

// Methods for BaseConfig that are promoted to DevEnvConfig

// GetSSHKeys returns the SSH public keys as a normalized string slice.
// It handles both single string and string array formats from the YAML
// configuration, converting them to a consistent []string format.
//
// Returns an error if the SSH key field contains invalid data types
// or empty key values.
func (c *BaseConfig) GetSSHKeys() ([]string, error) {
	return normalizeSSHKeys(c.SSHPublicKey)
}

// Methods for DevEnvConfig (these are NOT promoted from BaseConfig)

// GetDeveloperDir returns the filesystem path to the developer's configuration directory.
// This path is set during configuration loading and points to the directory containing
// the developer's devenv-config.yaml file and any associated resources.
func (c *DevEnvConfig) GetDeveloperDir() string {
	return c.DeveloperDir
}

// GetUserID returns the user ID as a string for use in Kubernetes manifests.
func (c *DevEnvConfig) GetUserID() string {
	return fmt.Sprintf("%d", c.UID)
}

// GPU returns the number of GPU resources requested for the developer environment.
// Returns 0 if no GPU allocation is specified in the configuration.
func (c *DevEnvConfig) GPU() int {
	if c.Resources.GPU < 0 {
		return 0
	}
	return c.Resources.GPU
}

// CPU returns the canonical CPU quantity formatted for Kubernetes (e.g., "2500m").
// Assumes Resources.CPU has already been normalized to millicores.
// Returns "0" if CPU <= 0 so callers can omit the field or treat as no request.
func (c *DevEnvConfig) CPU() string {
	CPU_in_millicores, err := c.Resources.getCanonicalCPU()
	if err != nil || CPU_in_millicores <= 0 {
		return "0"
	}
	return fmt.Sprintf("%dm", CPU_in_millicores)
}

// Memory returns "Gi" or "Mi" ("" means omit).
func (c *DevEnvConfig) Memory() string {
	memory_in_Mi, err := c.Resources.getCanonicalMemory()
	if err != nil || memory_in_Mi <= 0 {
		return ""
	}
	if memory_in_Mi%1024 == 0 {
		return fmt.Sprintf("%dGi", memory_in_Mi/1024)
	}
	return fmt.Sprintf("%dMi", memory_in_Mi)
}

// CPURequest returns the CPU resource request as a string suitable for Kubernetes manifests.
// This is currently an alias for the CPU method, but separated for potential future
// differentiation between limits and requests.
func (c *DevEnvConfig) CPURequest() string {
	return c.CPU()
}

// MemoryRequest returns the memory resource request as a string suitable for Kubernetes manifests.
// This is currently an alias for the Memory method, but separated for potential future
// differentiation between limits and requests.
func (c *DevEnvConfig) MemoryRequest() string {
	return c.Memory()
}

// NodePort returns the SSH port number for NodePort service configuration.
// This is an alias for the SSHPort field, providing template-friendly access
// to the port value for Kubernetes NodePort services.
func (c *DevEnvConfig) NodePort() int {
	return c.SSHPort
}

// VolumeMounts returns the configured volume mount specifications.
// Returns the slice of VolumeMount configurations for binding local directories
// into the developer environment container.
func (c *DevEnvConfig) VolumeMounts() []VolumeMount {
	if c.Volumes == nil {
		return nil
	}
	out := make([]VolumeMount, len(c.Volumes))
	copy(out, c.Volumes) // copies the slice AND each struct element by value
	return out
}

// GetSSHKeysSlice returns SSH keys as a string slice for use in Go templates.
// This method handles errors internally and returns an empty slice if SSH key
// parsing fails, making it safe for use in templates where error handling
// is not possible.
func (c *DevEnvConfig) GetSSHKeysSlice() []string {
	keys, err := c.GetSSHKeys()
	if err != nil || len(keys) == 0 {
		return []string{}
	}
	cp := make([]string, len(keys))
	copy(cp, keys)
	return cp
}

// GetSSHKeysString returns all SSH keys as a single newline-separated string
// for use in Go templates. This format is suitable for writing to authorized_keys
// files or similar multi-line SSH key configurations.
//
// Returns an empty string if SSH key parsing fails, making it safe for use
// in templates where error handling is not possible.
func (c *DevEnvConfig) GetSSHKeysString() string {
	keys := c.GetSSHKeysSlice()
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf("%s\n", strings.Join(keys, "\n"))
}

func (c *DevEnvConfig) Validate() error {
	return ValidateDevEnvConfig(c)
}
