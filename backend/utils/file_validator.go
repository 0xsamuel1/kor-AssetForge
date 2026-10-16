package utils

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// ValidateFile checks an uploaded file using magic bytes (not the extension),
// enforcing the allowed MIME-type whitelist and size limit (#332). Error
// messages never expose internals. The whitelist and limit are injected by the
// caller to avoid an import cycle with the config package.
func ValidateFile(file multipart.File, size int64, allowedTypes map[string]struct{}, maxSize int64) (mimeType string, err error) {
	if file == nil {
		return "", fmt.Errorf("no file provided")
	}
	if size > maxSize {
		return "", fmt.Errorf("file exceeds the maximum allowed size")
	}
	if size == 0 {
		return "", fmt.Errorf("empty file not allowed")
	}

	body := make([]byte, 512)
	if _, err := io.ReadFull(file, body); err != nil && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("cannot read uploaded file")
	}

	detected := http.DetectContentType(body)
	detected = strings.SplitN(detected, ";", 2)[0]

	if _, ok := allowedTypes[detected]; !ok {
		return "", fmt.Errorf("file type is not allowed")
	}

	return detected, nil
}
