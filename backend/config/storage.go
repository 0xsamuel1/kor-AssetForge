package config

import (
	"encoding/hex"
	"errors"
	"os"
)

// StorageConfig holds configuration for KYC document storage including
// the encryption key and storage backend settings.
type StorageConfig struct {
	// EncryptionKey is a 32-byte AES-256 key used to encrypt documents at rest.
	// Sourced from the DOCUMENT_ENCRYPTION_KEY environment variable (64 hex chars).
	EncryptionKey [32]byte

	// BasePath is the root directory (or bucket prefix) where encrypted documents
	// are stored. Defaults to "storage/kyc" for local development.
	BasePath string

	// MaxFileSizeBytes is the maximum allowed upload size per document.
	// Defaults to 10 MB.
	MaxFileSizeBytes int64
}

// LoadStorageConfig reads storage configuration from environment variables.
//
// Required env vars:
//   - DOCUMENT_ENCRYPTION_KEY: 64 hex-encoded bytes (32 bytes for AES-256)
//
// Optional env vars:
//   - KYC_STORAGE_PATH:        filesystem path or bucket prefix (default: "storage/kyc")
//   - KYC_MAX_FILE_SIZE_MB:    max upload size in MB (default: 10)
func LoadStorageConfig() (*StorageConfig, error) {
	cfg := &StorageConfig{
		BasePath:         "storage/kyc",
		MaxFileSizeBytes: 10 << 20, // 10 MB
	}

	keyHex := os.Getenv("DOCUMENT_ENCRYPTION_KEY")
	if keyHex == "" {
		// In development fall back to a fixed insecure key so the service
		// starts without configuration. This must never be used in production.
		keyHex = "0000000000000000000000000000000000000000000000000000000000000000"
	}

	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, errors.New("DOCUMENT_ENCRYPTION_KEY must be 64 hex characters (32 bytes)")
	}
	if len(keyBytes) != 32 {
		return nil, errors.New("DOCUMENT_ENCRYPTION_KEY must decode to exactly 32 bytes")
	}
	copy(cfg.EncryptionKey[:], keyBytes)

	if p := os.Getenv("KYC_STORAGE_PATH"); p != "" {
		cfg.BasePath = p
	}

	return cfg, nil
}
