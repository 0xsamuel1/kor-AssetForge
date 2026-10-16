package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/utils"
)

// FileUploadHandler validates uploaded files using magic-byte checks (#332).
type FileUploadHandler struct{}

// NewFileUploadHandler creates a FileUploadHandler.
func NewFileUploadHandler() *FileUploadHandler {
	return &FileUploadHandler{}
}

// Upload handles multipart file uploads.
// @Summary Upload a file
// @Description Upload and validate a file using magic-byte content-type checks
// @Tags upload
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /upload [post]
func (h *FileUploadHandler) Upload(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("file is required", err))
		return
	}

	f, err := fh.Open()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Unable to read uploaded file"))
		return
	}
	defer f.Close()

	mimeType, err := utils.ValidateFile(f, fh.Size, config.AllowedMIMETypes, config.MaxUploadFileSizeBytes)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid file upload"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "file validated",
		"file_name":  fh.Filename,
		"mime_type":  mimeType,
		"size_bytes": fh.Size,
	})
}
