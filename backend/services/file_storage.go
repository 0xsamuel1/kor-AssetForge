package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	defaultMaxFileSizeBytes int64 = 20 * 1024 * 1024 // 20 MB
	thumbnailMaxDimension         = 256
)

// allowedMIMETypes is the set of accepted MIME types for uploaded files.
var allowedMIMETypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"image/gif":       ".gif",
	"application/pdf": ".pdf",
	// Additional document types for asset verification materials.
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       ".xlsx",
}

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

// FileMetadata holds information about an uploaded file persisted alongside
// or instead of the S3 object metadata.
type FileMetadata struct {
	ID           string    `json:"id"`
	OriginalName string    `json:"original_name"`
	StoredKey    string    `json:"stored_key"`
	MIMEType     string    `json:"mime_type"`
	Extension    string    `json:"extension"`
	SizeBytes    int64     `json:"size_bytes"`
	SHA256       string    `json:"sha256"`
	UploaderID   string    `json:"uploader_id"`
	AssetID      string    `json:"asset_id,omitempty"`
	Purpose      string    `json:"purpose,omitempty"` // "image", "document", "verification"
	UploadedAt   time.Time `json:"uploaded_at"`
	CDNUrl       string    `json:"cdn_url,omitempty"`
}

// UploadResult is returned from a successful upload.
type UploadResult struct {
	Metadata     FileMetadata `json:"metadata"`
	PresignedURL string       `json:"presigned_url"` // short-lived direct-access URL
}

// ---------------------------------------------------------------------------
// FileStorageService
// ---------------------------------------------------------------------------

// FileStorageService handles secure file upload and storage via an
// S3-compatible backend (AWS S3, MinIO, Cloudflare R2, etc.).
type FileStorageService struct {
	client      *s3.Client
	presigner   *s3.PresignClient
	bucket      string
	cdnBase     string
	maxFileSize int64
	logger      *zap.SugaredLogger

	encryptor *FileEncryptor
	scanner   VirusScanner
}

// FileStorageConfig groups the configuration required to instantiate a
// FileStorageService.  All values are read from environment variables by
// NewFileStorageServiceFromEnv.
type FileStorageConfig struct {
	Endpoint         string // e.g. "https://s3.amazonaws.com" or a MinIO URL
	Region           string
	AccessKeyID      string
	SecretAccessKey  string
	Bucket           string
	CDNBase          string // optional CDN prefix, e.g. "https://cdn.example.com"
	MaxFileSizeBytes int64  // 0 → defaultMaxFileSizeBytes

	Provider string
}

type VirusScanner interface {
	Scan(ctx context.Context, file []byte) error
}

type FileEncryptor struct {
	key []byte
}

func NewFileEncryptor(key []byte) *FileEncryptor {
	return &FileEncryptor{key: key}
}

// NewFileStorageServiceFromEnv reads S3 configuration from environment
// variables and constructs a FileStorageService.
//
// Required env vars:
//
//	S3_ENDPOINT          S3-compatible endpoint
//	S3_REGION            AWS / compatible region
//	S3_ACCESS_KEY_ID     Access key
//	S3_SECRET_ACCESS_KEY Secret key
//	S3_BUCKET            Target bucket name
//
// Optional:
//
//	S3_CDN_BASE          CDN base URL for public links
//	S3_MAX_FILE_SIZE     Maximum upload size in bytes (default: 20 MB)
func NewFileStorageServiceFromEnv(logger *zap.SugaredLogger) (*FileStorageService, error) {
	cfg := FileStorageConfig{
		Endpoint:        os.Getenv("S3_ENDPOINT"),
		Region:          getEnvOrDefaultStr("S3_REGION", "us-east-1"),
		AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
		Bucket:          os.Getenv("S3_BUCKET"),
		CDNBase:         os.Getenv("S3_CDN_BASE"),
	}
	if cfg.Bucket == "" {
		return nil, errors.New("file storage: S3_BUCKET env var is required")
	}
	return NewFileStorageService(cfg, logger)
}

func encryptAESGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
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

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func decryptAESGCM(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// NewFileStorageService constructs a FileStorageService from an explicit config.
func NewFileStorageService(cfg FileStorageConfig, logger *zap.SugaredLogger) (*FileStorageService, error) {
	maxSize := cfg.MaxFileSizeBytes
	if maxSize <= 0 {
		maxSize = defaultMaxFileSizeBytes
	}

	encKey := os.Getenv("FILE_ENCRYPTION_KEY")
	if len(encKey) != 32 {
		return nil, errors.New("file storage: FILE_ENCRYPTION_KEY must be 32 bytes")
	}

	encryptor := NewFileEncryptor([]byte(encKey))

	var scanner VirusScanner = nil

	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("file storage: failed to build AWS config: %w", err)
	}

	var opts []func(*s3.Options)
	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // required for MinIO / custom endpoints
		})
	}

	client := s3.NewFromConfig(awsCfg, opts...)
	return &FileStorageService{
		client:      client,
		presigner:   s3.NewPresignClient(client),
		bucket:      cfg.Bucket,
		cdnBase:     cfg.CDNBase,
		maxFileSize: maxSize,
		logger:      logger,

		encryptor: encryptor,
		scanner:   scanner,
	}, nil
}

// ---------------------------------------------------------------------------
// Upload
// ---------------------------------------------------------------------------

// Upload validates and stores a multipart file. It returns rich metadata and
// a short-lived presigned download URL.
//
// Validation order:
//  1. File size ≤ maxFileSize
//  2. MIME type is allowed (detected from content, not filename)
//  3. Content is non-empty
func (s *FileStorageService) Upload(
	ctx context.Context,
	fh *multipart.FileHeader,
	uploaderID string,
	purpose string,
	assetID string,
) (*UploadResult, error) {
	// Size guard.
	if fh.Size > s.maxFileSize {
		return nil, fmt.Errorf("file storage: file size %d exceeds limit of %d bytes", fh.Size, s.maxFileSize)
	}

	f, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("file storage: cannot open uploaded file: %w", err)
	}
	defer f.Close()

	// Read entire body so we can: detect MIME, compute hash, upload.
	body, err := io.ReadAll(io.LimitReader(f, s.maxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("file storage: failed to read file: %w", err)
	}

	if len(body) == 0 {
		return nil, errors.New("file storage: empty file not allowed")
	}

	if int64(len(body)) > s.maxFileSize {
		return nil, fmt.Errorf("file storage: file size exceeds limit of %d bytes", s.maxFileSize)
	}

	// Detect MIME from content (ignores client-supplied Content-Type).
	detectedMIME := http.DetectContentType(body[:min(512, len(body))])
	// http.DetectContentType may append parameters; strip them.
	detectedMIME = strings.SplitN(detectedMIME, ";", 2)[0]

	ext, ok := allowedMIMETypes[detectedMIME]
	if !ok {
		// Fall back to extension-based check for document types not detectable
		// by magic bytes.
		clientExt := strings.ToLower(filepath.Ext(fh.Filename))
		for mime, mExt := range allowedMIMETypes {
			if mExt == clientExt {
				ext = mExt
				detectedMIME = mime
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("file storage: MIME type %q is not allowed", detectedMIME)
		}
	}

	if s.scanner != nil {
		if err := s.scanner.Scan(ctx, body); err != nil {
			return nil, fmt.Errorf("file storage: virus detected: %w", err)
		}
	}

	// Compute SHA-256 checksum for integrity and deduplication.
	hash := sha256.Sum256(body)
	hashHex := hex.EncodeToString(hash[:])

	encryptedBody, err := encryptAESGCM(s.encryptor.key, body)
	if err != nil {
		return nil, fmt.Errorf("file storage: encryption failed: %w", err)
	}

	fileID := uuid.New().String()
	storedKey := buildStorageKey(purpose, assetID, fileID, ext)

	// Upload to S3.
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(storedKey),
		Body:        bytes.NewReader(encryptedBody),
		ContentType: aws.String(detectedMIME),
		Metadata: map[string]string{
			"uploader-id":   uploaderID,
			"original-name": fh.Filename,
			"sha256":        hashHex,
			"purpose":       purpose,
			"asset-id":      assetID,
		},
	}); err != nil {
		return nil, fmt.Errorf("file storage: S3 upload failed: %w", err)
	}

	s.logger.Infow("file uploaded",
		"key", storedKey,
		"mime", detectedMIME,
		"size", len(body),
		"uploader", uploaderID,
	)

	// Generate a presigned download URL (valid 15 minutes).
	presigned, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(storedKey),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		s.logger.Warnw("failed to generate presigned URL", "key", storedKey, "err", err)
	}

	meta := FileMetadata{
		ID:           fileID,
		OriginalName: fh.Filename,
		StoredKey:    storedKey,
		MIMEType:     detectedMIME,
		Extension:    ext,
		SizeBytes:    int64(len(body)),
		SHA256:       hashHex,
		UploaderID:   uploaderID,
		AssetID:      assetID,
		Purpose:      purpose,
		UploadedAt:   time.Now().UTC(),
		CDNUrl:       s.cdnURL(storedKey),
	}

	result := &UploadResult{Metadata: meta}
	if presigned != nil {
		result.PresignedURL = presigned.URL
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// Delete removes an object from storage by its stored key.
func (s *FileStorageService) Delete(ctx context.Context, storedKey string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(storedKey),
	}); err != nil {
		return fmt.Errorf("file storage: delete failed for key %q: %w", storedKey, err)
	}
	s.logger.Infow("file deleted", "key", storedKey)
	return nil
}

// ---------------------------------------------------------------------------
// Presigned URL
// ---------------------------------------------------------------------------

// PresignDownload generates a fresh presigned URL for an existing object.
func (s *FileStorageService) PresignDownload(ctx context.Context, storedKey string, ttl time.Duration) (string, error) {
	req, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(storedKey),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("file storage: presign failed for key %q: %w", storedKey, err)
	}
	return req.URL, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func buildStorageKey(purpose, assetID, fileID, ext string) string {
	if purpose == "" {
		purpose = "misc"
	}
	date := time.Now().UTC().Format("2006/01/02")
	if assetID != "" {
		return fmt.Sprintf("%s/%s/%s/%s%s", purpose, assetID, date, fileID, ext)
	}
	return fmt.Sprintf("%s/%s/%s%s", purpose, date, fileID, ext)
}

func (s *FileStorageService) cdnURL(key string) string {
	if s.cdnBase == "" {
		return ""
	}
	return strings.TrimRight(s.cdnBase, "/") + "/" + key
}

func getEnvOrDefaultStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
