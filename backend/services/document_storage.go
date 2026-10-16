package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/kor-assetforge/config"
)

// DocumentStorageService handles secure storage of KYC documents with
// AES-256-GCM encryption at rest. Documents are stored under:
//
//	<basePath>/kyc/<userID>/<kycRecordID>/<documentType>/<uuid>.enc
//
// The raw file bytes are never persisted unencrypted. Only the storage path
// and a SHA-256 hash of the original bytes are kept in the database
// (models.KYCDocument).
type DocumentStorageService struct {
	cfg *config.StorageConfig
}

// NewDocumentStorageService returns a DocumentStorageService using the provided
// StorageConfig. Call config.LoadStorageConfig() to obtain a production config.
func NewDocumentStorageService(cfg *config.StorageConfig) *DocumentStorageService {
	if cfg == nil {
		// Fall back to default dev config when called with nil.
		cfg, _ = config.LoadStorageConfig()
	}
	return &DocumentStorageService{cfg: cfg}
}

// StoreDocument encrypts data and writes it to the storage path derived from the
// provided identifiers. It returns the storage path to be persisted in the DB.
//
// The nonce is prepended to the ciphertext so that it can be recovered during
// decryption without additional metadata storage.
func (s *DocumentStorageService) StoreDocument(
	userID uint,
	kycRecordID uint,
	documentType string,
	data []byte,
) (storagePath string, err error) {
	if len(data) == 0 {
		return "", errors.New("document data must not be empty")
	}
	if int64(len(data)) > s.cfg.MaxFileSizeBytes {
		return "", fmt.Errorf("document exceeds maximum allowed size of %d bytes", s.cfg.MaxFileSizeBytes)
	}

	encrypted, err := s.encrypt(data)
	if err != nil {
		return "", fmt.Errorf("encryption failed: %w", err)
	}

	relPath := filepath.Join(
		"kyc",
		fmt.Sprintf("%d", userID),
		fmt.Sprintf("%d", kycRecordID),
		documentType,
		uuid.NewString()+".enc",
	)
	fullPath := filepath.Join(s.cfg.BasePath, relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		return "", fmt.Errorf("failed to create storage directory: %w", err)
	}

	if err := os.WriteFile(fullPath, encrypted, 0o600); err != nil {
		return "", fmt.Errorf("failed to write document: %w", err)
	}

	return relPath, nil
}

// RetrieveDocument reads and decrypts the document at the given storage path.
// The path is a relative path as returned by StoreDocument, resolved against
// the configured BasePath.
func (s *DocumentStorageService) RetrieveDocument(storagePath string) ([]byte, error) {
	fullPath := filepath.Join(s.cfg.BasePath, storagePath)

	ciphertext, err := os.ReadFile(fullPath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to read document: %w", err)
	}

	return s.decrypt(ciphertext)
}

// DeleteDocument removes the encrypted document from storage. It does NOT
// remove the KYCDocument database record — the caller is responsible for that.
func (s *DocumentStorageService) DeleteDocument(storagePath string) error {
	fullPath := filepath.Join(s.cfg.BasePath, storagePath)
	if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to delete document: %w", err)
	}
	return nil
}

// PurgeExpired deletes encrypted document files whose retention deadline has
// passed. olderThan is compared against the file's modification time. This is
// intended to be called from a scheduled job.
//
// Returns the number of files deleted and any error encountered.
func (s *DocumentStorageService) PurgeExpired(olderThan time.Duration) (int, error) {
	kycRoot := filepath.Join(s.cfg.BasePath, "kyc")
	deleted := 0

	err := filepath.WalkDir(kycRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if time.Since(info.ModTime()) > olderThan {
			if removeErr := os.Remove(path); removeErr == nil {
				deleted++
			}
		}
		return nil
	})

	return deleted, err
}

// ReEncrypt re-encrypts a document with the current key. This is used during
// key rotation: read with the old key (passed explicitly), re-encrypt with the
// current service key, and overwrite in place.
func (s *DocumentStorageService) ReEncrypt(storagePath string, oldKey [32]byte) error {
	fullPath := filepath.Join(s.cfg.BasePath, storagePath)

	ciphertext, err := os.ReadFile(fullPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to read document for re-encryption: %w", err)
	}

	plain, err := decryptWithKey(ciphertext, oldKey)
	if err != nil {
		return fmt.Errorf("decryption with old key failed: %w", err)
	}

	newCiphertext, err := s.encrypt(plain)
	if err != nil {
		return fmt.Errorf("re-encryption failed: %w", err)
	}

	if err := os.WriteFile(fullPath, newCiphertext, 0o600); err != nil {
		return fmt.Errorf("failed to write re-encrypted document: %w", err)
	}

	return nil
}

// ---- internal helpers -------------------------------------------------------

// encrypt encrypts plaintext using AES-256-GCM with the service's key.
// The output format is: [12-byte nonce][ciphertext+tag].
func (s *DocumentStorageService) encrypt(plaintext []byte) ([]byte, error) {
	return encryptWithKey(plaintext, s.cfg.EncryptionKey)
}

func encryptWithKey(plaintext []byte, key [32]byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return sealed, nil
}

// decrypt decrypts data encrypted by encrypt() using the service's current key.
func (s *DocumentStorageService) decrypt(ciphertext []byte) ([]byte, error) {
	return decryptWithKey(ciphertext, s.cfg.EncryptionKey)
}

func decryptWithKey(ciphertext []byte, key [32]byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
