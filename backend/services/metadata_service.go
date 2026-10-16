package services

import (
	"errors"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// MetadataService manages versioned metadata changes for assets.
type MetadataService struct {
	db *gorm.DB
}

// NewMetadataService creates a new MetadataService.
func NewMetadataService(db *gorm.DB) *MetadataService {
	return &MetadataService{db: db}
}

// RecordVersion snapshots the current metadata state of an asset before a change.
// It auto-increments the version number per asset.
func (s *MetadataService) RecordVersion(asset models.Asset, changedBy uint, changeNote string) error {
	var latest models.MetadataVersion
	var nextVersion int = 1

	err := s.db.Where("asset_id = ?", asset.ID).
		Order("version desc").
		First(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		nextVersion = latest.Version + 1
	}

	version := models.MetadataVersion{
		AssetID:      asset.ID,
		Version:      nextVersion,
		MetadataURI:  asset.MetadataURI,
		MetadataHash: asset.MetadataHash,
		Metadata:     asset.Metadata,
		ChangedBy:    changedBy,
		ChangeNote:   changeNote,
	}

	return s.db.Create(&version).Error
}

// ListVersions returns all recorded versions for an asset, newest first.
func (s *MetadataService) ListVersions(assetID uint) ([]models.MetadataVersion, error) {
	var versions []models.MetadataVersion
	err := s.db.Where("asset_id = ?", assetID).
		Order("version desc").
		Find(&versions).Error
	return versions, err
}

// GetVersion returns a specific version for an asset.
func (s *MetadataService) GetVersion(assetID uint, version int) (models.MetadataVersion, error) {
	var v models.MetadataVersion
	err := s.db.Where("asset_id = ? AND version = ?", assetID, version).First(&v).Error
	return v, err
}

// RevertToVersion restores an asset's metadata fields to a prior version snapshot.
// It records the revert itself as a new version entry.
func (s *MetadataService) RevertToVersion(asset *models.Asset, version int, revertedBy uint) error {
	v, err := s.GetVersion(asset.ID, version)
	if err != nil {
		return err
	}

	if err := s.RecordVersion(*asset, revertedBy, "pre-revert snapshot"); err != nil {
		return err
	}

	asset.MetadataURI = v.MetadataURI
	asset.MetadataHash = v.MetadataHash
	asset.Metadata = v.Metadata

	return s.db.Save(asset).Error
}
