package handlers

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/utils"
)

var allowedTypes = config.AllowedMIMETypes
var maxSize = config.MaxUploadFileSizeBytes

func tempFile(t *testing.T, name string, content []byte) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	require.NoError(t, err)
	_, err = f.Write(content)
	require.NoError(t, err)
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	return f
}

func TestValidateFileAcceptsJpegByMagicBytes(t *testing.T) {
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 100)...)
	f := tempFile(t, "photo.jpg", jpeg)
	defer f.Close()

	mime, err := utils.ValidateFile(f, int64(len(jpeg)), config.AllowedMIMETypes, config.MaxUploadFileSizeBytes)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", mime)
}

func TestValidateFileRejectsTextFile(t *testing.T) {
	content := []byte("hello world")
	f := tempFile(t, "note.txt", content)
	defer f.Close()

	_, err := utils.ValidateFile(f, int64(len(content)), allowedTypes, maxSize)
	assert.Error(t, err)
}

func TestValidateFileRejectsEmptyFile(t *testing.T) {
	f := tempFile(t, "empty.pdf", nil)
	defer f.Close()

	_, err := utils.ValidateFile(f, 0, allowedTypes, maxSize)
	assert.Error(t, err)
}

func TestValidateFileRejectsOversizedFile(t *testing.T) {
	big := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF}, 4*1024*1024)
	f := tempFile(t, "big.jpg", big)
	defer f.Close()

	_, err := utils.ValidateFile(f, int64(len(big)), allowedTypes, maxSize)
	assert.Error(t, err)
}
