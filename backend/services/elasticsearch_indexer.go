package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// ElasticsearchIndexer handles indexing of assets into Elasticsearch
type ElasticsearchIndexer struct {
	client    *elasticsearch.Client
	indexName string
	db        *gorm.DB
}

// NewElasticsearchIndexer creates a new indexer instance
func NewElasticsearchIndexer(client *elasticsearch.Client, db *gorm.DB) *ElasticsearchIndexer {
	return &ElasticsearchIndexer{
		client:    client,
		indexName: "assets",
		db:        db,
	}
}

// IndexAssetsBulk performs bulk indexing of assets
func (i *ElasticsearchIndexer) IndexAssetsBulk(ctx context.Context, assets []*models.Asset) error {
	if i.client == nil || len(assets) == 0 {
		return nil
	}

	var bulkRequest strings.Builder

	for _, asset := range assets {
		// Index metadata line
		bulkRequest.WriteString(fmt.Sprintf(`{"index":{"_index":"%s","_id":"%d"}}`, i.indexName, asset.ID))
		bulkRequest.WriteString("\n")

		// Document line
		doc := i.assetToDocument(asset)
		docJSON, _ := json.Marshal(doc)
		bulkRequest.Write(docJSON)
		bulkRequest.WriteString("\n")
	}

	res, err := i.client.Bulk(
		strings.NewReader(bulkRequest.String()),
		i.client.Bulk.WithContext(ctx),
	)

	if err != nil || res.IsError() {
		return fmt.Errorf("bulk indexing failed: %w", err)
	}

	return nil
}

// ReindexAllAssets reindexes all assets from the database
func (i *ElasticsearchIndexer) ReindexAllAssets(ctx context.Context) error {
	if i.client == nil {
		return nil
	}

	// Delete old index
	i.client.Indices.Delete([]string{i.indexName})
	time.Sleep(500 * time.Millisecond)

	// Recreate index with mappings
	if err := i.createIndex(ctx); err != nil {
		return err
	}

	// Fetch all assets
	var assets []*models.Asset
	if err := i.db.Where("deleted_at IS NULL").FindInBatches(&assets, 1000, func(tx *gorm.DB, batch int) error {
		return i.IndexAssetsBulk(ctx, assets)
	}).Error; err != nil {
		return fmt.Errorf("failed to fetch assets for reindexing: %w", err)
	}

	return nil
}

// createIndex creates the Elasticsearch index with proper mappings
func (i *ElasticsearchIndexer) createIndex(ctx context.Context) error {
	if i.client == nil {
		return nil
	}

	mapping := map[string]interface{}{
		"settings": map[string]interface{}{
			"number_of_shards":   1,
			"number_of_replicas": 0,
			"analysis": map[string]interface{}{
				"analyzer": map[string]interface{}{
					"asset_analyzer": map[string]interface{}{
						"type":      "custom",
						"tokenizer": "standard",
						"filter": []string{
							"lowercase",
							"stop",
							"snowball",
						},
					},
				},
			},
		},
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"id": map[string]interface{}{"type": "keyword"},
				"name": map[string]interface{}{
					"type":     "text",
					"analyzer": "asset_analyzer",
					"fields": map[string]interface{}{
						"keyword":    map[string]interface{}{"type": "keyword"},
						"completion": map[string]interface{}{"type": "completion"},
					},
				},
				"symbol":           map[string]interface{}{"type": "keyword"},
				"description":      map[string]interface{}{"type": "text", "analyzer": "asset_analyzer"},
				"asset_type":       map[string]interface{}{"type": "keyword"},
				"total_supply":     map[string]interface{}{"type": "long"},
				"fractions":        map[string]interface{}{"type": "long"},
				"contract_id":      map[string]interface{}{"type": "keyword"},
				"owner_address":    map[string]interface{}{"type": "keyword"},
				"metadata":         map[string]interface{}{"type": "object", "enabled": false},
				"image_url":        map[string]interface{}{"type": "keyword"},
				"document_url":     map[string]interface{}{"type": "keyword"},
				"verified":         map[string]interface{}{"type": "boolean"},
				"created_at":       map[string]interface{}{"type": "date"},
				"updated_at":       map[string]interface{}{"type": "date"},
				"search_text":      map[string]interface{}{"type": "text", "analyzer": "asset_analyzer"},
				"categories":       map[string]interface{}{"type": "text", "analyzer": "asset_analyzer", "fields": map[string]interface{}{"keyword": map[string]interface{}{"type": "keyword"}}},
				"tags":             map[string]interface{}{"type": "text", "analyzer": "asset_analyzer", "fields": map[string]interface{}{"keyword": map[string]interface{}{"type": "keyword"}}},
				"popularity_score": map[string]interface{}{"type": "double"},
			},
		},
	}

	body, _ := json.Marshal(mapping)
	res, err := i.client.Indices.Create(
		i.indexName,
		i.client.Indices.Create.WithBody(strings.NewReader(string(body))),
	)

	if err != nil || res.IsError() {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return nil
}

// assetToDocument converts an Asset model to an Elasticsearch document
func (i *ElasticsearchIndexer) assetToDocument(asset *models.Asset) AssetDocument {
	var metadata map[string]string
	if err := json.Unmarshal([]byte(asset.Metadata), &metadata); err != nil {
		metadata = make(map[string]string)
	}

	searchText := fmt.Sprintf("%s %s %s %s", asset.Name, asset.Symbol, asset.Description, asset.AssetType)

	doc := AssetDocument{
		ID:           asset.ID,
		Name:         asset.Name,
		Symbol:       asset.Symbol,
		Description:  asset.Description,
		AssetType:    asset.AssetType,
		TotalSupply:  asset.TotalSupply,
		Fractions:    asset.Fractions,
		ContractID:   asset.ContractID,
		OwnerAddress: asset.OwnerAddress,
		Metadata:     metadata,
		ImageURL:     asset.ImageURL,
		DocumentURL:  asset.DocumentURL,
		Verified:     asset.Verified,
		CreatedAt:    asset.CreatedAt,
		UpdatedAt:    asset.UpdatedAt,
		SearchText:   searchText,
	}
	if i.db != nil && asset.ID != 0 {
		i.db.Model(&models.Category{}).
			Joins("JOIN asset_categories ON asset_categories.category_id = categories.id").
			Where("asset_categories.asset_id = ?", asset.ID).Pluck("categories.slug", &doc.Categories)
		i.db.Model(&models.Tag{}).
			Joins("JOIN asset_tags ON asset_tags.tag_id = tags.id").
			Where("asset_tags.asset_id = ?", asset.ID).Pluck("tags.slug", &doc.Tags)
		var interactions int64
		i.db.Model(&models.UserInteraction{}).Where("asset_id = ?", asset.ID).Count(&interactions)
		doc.PopularityScore = float64(interactions)
	}
	return doc
}
