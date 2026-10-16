package config

import "os"

// Environment represents the deployment environment the API is running in.
type Environment string

const (
	// EnvProduction is the live production environment.
	EnvProduction Environment = "production"
	// EnvStaging is the pre-production staging environment.
	EnvStaging Environment = "staging"
	// EnvSandbox is the developer-facing sandbox environment.
	// In sandbox mode all external side effects are suppressed:
	// no real payments are processed, no real emails sent, and blockchain
	// calls are directed at the Stellar testnet.
	EnvSandbox Environment = "sandbox"
	// EnvDevelopment is the local development environment.
	EnvDevelopment Environment = "development"
	// EnvTest is used during automated test runs.
	EnvTest Environment = "test"
)

// CurrentEnvironment reads the APP_ENV environment variable and returns the
// corresponding Environment constant. Defaults to EnvDevelopment when unset.
func CurrentEnvironment() Environment {
	switch os.Getenv("APP_ENV") {
	case "production":
		return EnvProduction
	case "staging":
		return EnvStaging
	case "sandbox":
		return EnvSandbox
	case "test":
		return EnvTest
	default:
		return EnvDevelopment
	}
}

// IsSandbox reports whether the application is running in sandbox mode.
// Use this guard anywhere you need to suppress real external calls.
func IsSandbox() bool {
	return CurrentEnvironment() == EnvSandbox
}

// IsProduction reports whether the application is running in production mode.
func IsProduction() bool {
	return CurrentEnvironment() == EnvProduction
}

// SandboxConfig holds configuration values specific to the sandbox environment.
type SandboxConfig struct {
	// SeededUserEmail is the email address of the pre-seeded sandbox admin user.
	SeededUserEmail string
	// SeededUserPassword is the password for the pre-seeded sandbox admin user.
	SeededUserPassword string
	// ResetEnabled controls whether the /sandbox/reset endpoint is active.
	ResetEnabled bool
}

// LoadSandboxConfig reads sandbox-specific configuration from the environment.
func LoadSandboxConfig() SandboxConfig {
	return SandboxConfig{
		SeededUserEmail:    getEnvOrDefaultStr("SANDBOX_ADMIN_EMAIL", "sandbox@kor-assetforge.dev"),
		SeededUserPassword: getEnvOrDefaultStr("SANDBOX_ADMIN_PASSWORD", "SandboxPass123!"),
		ResetEnabled:       os.Getenv("SANDBOX_RESET_ENABLED") != "false",
	}
}
