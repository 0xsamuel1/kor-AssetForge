package config

import (
	"os"
	"strconv"
)

// AllowedMIMETypes is the whitelist of MIME types accepted for file uploads (#332).
// Validation is performed against magic bytes, never the file extension.
var AllowedMIMETypes = map[string]struct{}{
	"image/jpeg":                   {},
	"image/png":                    {},
	"image/webp":                   {},
	"image/gif":                    {},
	"application/pdf":              {},
	"application/msword":           {},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {},
}

// MaxUploadFileSizeBytes is the maximum allowed upload size (10 MB).
var MaxUploadFileSizeBytes int64 = 10 * 1024 * 1024

func init() {
	if v, err := strconv.ParseInt(os.Getenv("MAX_UPLOAD_FILE_SIZE_BYTES"), 10, 64); err == nil && v > 0 {
		MaxUploadFileSizeBytes = v
	}
}
