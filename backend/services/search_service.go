package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// SearchRequest contains all filter / sort / pagination parameters.
type SearchRequest struct {
	Query       string            `form:"q"`
	AssetType   string            `form:"asset_type"` // compatibility with a single-value client
	AssetTypes  []string          `form:"asset_type"`
	MinPrice    *int64            `form:"min_price"`
	MaxPrice    *int64            `form:"max_price"`
	Location    string            `form:"location"`
	Locations   []string          `form:"location"`
	CreatedFrom *time.Time        `form:"created_from" time_format:"2006-01-02"`
	CreatedTo   *time.Time        `form:"created_to" time_format:"2006-01-02"`
	Verified    *bool             `form:"verified"`
	Metadata    map[string]string `form:"-"`
	SortBy      string            `form:"sort_by"` // name | created_at | total_supply | fractions
	Order       string            `form:"order"`   // asc | desc
	Page        int               `form:"page"`
	Limit       int               `form:"limit"`
}

// SearchResult is the paginated response envelope.
type SearchResult struct {
	Total            int64          `json:"total"`
	Page             int            `json:"page"`
	Limit            int            `json:"limit"`
	Assets           []models.Asset `json:"assets"`
	Facets           SearchFacets   `json:"facets"`
	Took             float64        `json:"took_ms"` // query duration in milliseconds
	AnalyticsEventID string         `json:"analytics_event_id,omitempty"`
}

// SearchFacets carries aggregated filter counts for faceted navigation.
type SearchFacets struct {
	AssetTypes []FacetBucket `json:"asset_types"`
	Locations  []FacetBucket `json:"locations"`
	Verified   []FacetBucket `json:"verified"`
}

// FacetBucket is a single facet value with its document count.
type FacetBucket struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// SuggestResult holds lightweight search suggestions.
type SuggestResult struct {
	Suggestions []string `json:"suggestions"`
}

// SearchAnalyticsEvent is appended to the analytics log on every search.
type SearchAnalyticsEvent struct {
	EventID     string                 `json:"event_id"`
	Query       string                 `json:"query"`
	Filters     map[string]interface{} `json:"filters"`
	ResultCount int64                  `json:"result_count"`
	TookMs      float64                `json:"took_ms"`
	UserID      string                 `json:"user_id,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// ---- SearchBackend interface -------------------------------------------------

// SearchBackend abstracts the underlying search engine.
// DBSearchBackend (PostgreSQL) is the default; ESSearchBackend integrates
// Elasticsearch with transparent fallback to DB when ES is unavailable.
type SearchBackend interface {
	Search(ctx context.Context, req *SearchRequest) (*SearchResult, error)
	Suggest(ctx context.Context, query string, limit int) (*SuggestResult, error)
	IndexAsset(ctx context.Context, asset *models.Asset) error
	DeleteAssetFromIndex(ctx context.Context, assetID uint) error
	ReindexAll(ctx context.Context) error
	RecordAnalytics(ctx context.Context, event *SearchAnalyticsEvent) error
}

// ---- DBSearchBackend --------------------------------------------------------

// DBSearchBackend uses PostgreSQL ILIKE full-text search and GORM scopes.
// No external dependencies — works out of the box.
type DBSearchBackend struct {
	db *gorm.DB
}

// NewDBSearchBackend constructs a DBSearchBackend.
func NewDBSearchBackend(db *gorm.DB) SearchBackend { return &DBSearchBackend{db: db} }

func (s *DBSearchBackend) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	req.normalizePagination()
	q := s.db.WithContext(ctx).Model(&models.Asset{})

	// Full-text filter across name, symbol, description, asset_type
	if term := strings.TrimSpace(req.Query); term != "" {
		like := "%" + term + "%"
		q = q.Where(
			"name ILIKE ? OR symbol ILIKE ? OR description ILIKE ? OR asset_type ILIKE ?",
			like, like, like, like,
		)
	}

	// Exact asset-type filter
	if types := req.assetTypes(); len(types) > 0 {
		q = q.Where("asset_type IN ?", types)
	}

	// Verified flag
	if req.Verified != nil {
		q = q.Where("verified = ?", *req.Verified)
	}
	if locations := req.locations(); len(locations) > 0 {
		q = q.Where("NULLIF(metadata, '')::jsonb ->> 'location' IN ?", locations)
	}
	if req.CreatedFrom != nil {
		q = q.Where("created_at >= ?", req.CreatedFrom.UTC())
	}
	if req.CreatedTo != nil {
		q = q.Where("created_at < ?", req.CreatedTo.UTC().Add(24*time.Hour))
	}
	for key, value := range req.Metadata {
		metadataFilter, _ := json.Marshal(map[string]string{key: value})
		q = q.Where("NULLIF(metadata, '')::jsonb @> ?::jsonb", string(metadataFilter))
	}

	// Price range via subquery on active listings — avoids duplicate rows from JOIN
	if req.MinPrice != nil || req.MaxPrice != nil {
		subQ := s.db.Model(&models.Listing{}).Select("asset_id").
			Where("active = ? AND deleted_at IS NULL", true)
		if req.MinPrice != nil {
			subQ = subQ.Where("price_per_unit >= ?", *req.MinPrice)
		}
		if req.MaxPrice != nil {
			subQ = subQ.Where("price_per_unit <= ?", *req.MaxPrice)
		}
		q = q.Where("id IN (?)", subQ)
	}

	// Count before pagination
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("search count: %w", err)
	}

	// Sort
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	allowedSort := map[string]bool{"name": true, "created_at": true, "total_supply": true, "fractions": true}
	if !allowedSort[sortBy] {
		sortBy = "created_at"
	}
	order := strings.ToLower(req.Order)
	if order != "asc" {
		order = "desc"
	}
	q = q.Order(sortBy + " " + order)

	// Pagination
	page, limit := req.Page, req.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	q = q.Offset((page - 1) * limit).Limit(limit)

	var assets []models.Asset
	if err := q.Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}

	facets := s.buildFacets(ctx, req)

	return &SearchResult{
		Total:  total,
		Page:   page,
		Limit:  limit,
		Assets: assets,
		Facets: facets,
	}, nil
}

// Suggest returns auto-complete suggestions based on asset names and symbols.
func (s *DBSearchBackend) Suggest(ctx context.Context, query string, limit int) (*SuggestResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	term := strings.TrimSpace(query)
	if term == "" {
		return &SuggestResult{Suggestions: []string{}}, nil
	}
	like := term + "%"

	// Raw SQL with DISTINCT avoids GORM ambiguity and works across all drivers.
	type row struct{ Name string }
	var names []row
	s.db.WithContext(ctx).
		Raw("SELECT DISTINCT name FROM assets WHERE deleted_at IS NULL AND (name ILIKE ? OR symbol ILIKE ?) LIMIT ?",
			like, like, limit).
		Scan(&names)

	suggestions := make([]string, 0, len(names))
	for _, r := range names {
		suggestions = append(suggestions, r.Name)
	}
	return &SuggestResult{Suggestions: suggestions}, nil
}

func (s *DBSearchBackend) buildFacets(ctx context.Context, req *SearchRequest) SearchFacets {
	type typeBucket struct {
		AssetType string
		Count     int64
	}
	var buckets []typeBucket
	s.db.WithContext(ctx).Model(&models.Asset{}).
		Select("asset_type, COUNT(*) as count").
		Group("asset_type").
		Scan(&buckets)

	var verifiedCount, unverifiedCount int64
	s.db.WithContext(ctx).Model(&models.Asset{}).Where("verified = ?", true).Count(&verifiedCount)
	s.db.WithContext(ctx).Model(&models.Asset{}).Where("verified = ?", false).Count(&unverifiedCount)

	typeFacets := make([]FacetBucket, 0, len(buckets))
	for _, b := range buckets {
		typeFacets = append(typeFacets, FacetBucket{Value: b.AssetType, Count: b.Count})
	}
	type locationBucket struct {
		Location string
		Count    int64
	}
	var locations []locationBucket
	s.db.WithContext(ctx).Model(&models.Asset{}).Where("metadata IS NOT NULL AND metadata <> ''").
		Select("NULLIF(metadata, '')::jsonb ->> 'location' as location, COUNT(*) as count").Where("NULLIF(metadata, '')::jsonb ->> 'location' IS NOT NULL").Group("location").Scan(&locations)
	locationFacets := make([]FacetBucket, 0, len(locations))
	for _, b := range locations {
		locationFacets = append(locationFacets, FacetBucket{Value: b.Location, Count: b.Count})
	}
	sort.Slice(typeFacets, func(i, j int) bool { return typeFacets[i].Value < typeFacets[j].Value })
	sort.Slice(locationFacets, func(i, j int) bool { return locationFacets[i].Value < locationFacets[j].Value })

	_ = req
	return SearchFacets{
		AssetTypes: typeFacets,
		Locations:  locationFacets,
		Verified:   []FacetBucket{{Value: "true", Count: verifiedCount}, {Value: "false", Count: unverifiedCount}},
	}
}

func (req *SearchRequest) Validate() error {
	if req.MinPrice != nil && *req.MinPrice < 0 {
		return fmt.Errorf("min_price must not be negative")
	}
	if req.MaxPrice != nil && *req.MaxPrice < 0 {
		return fmt.Errorf("max_price must not be negative")
	}
	if req.MinPrice != nil && req.MaxPrice != nil && *req.MinPrice > *req.MaxPrice {
		return fmt.Errorf("min_price cannot exceed max_price")
	}
	if req.CreatedFrom != nil && req.CreatedTo != nil && req.CreatedFrom.After(*req.CreatedTo) {
		return fmt.Errorf("created_from cannot be after created_to")
	}
	if len(req.Metadata) > 10 {
		return fmt.Errorf("at most 10 metadata filters are allowed")
	}
	for key, value := range req.Metadata {
		if strings.TrimSpace(key) == "" || len(key) > 64 || len(value) > 256 {
			return fmt.Errorf("invalid metadata filter")
		}
	}
	if req.SortBy != "" && !map[string]bool{"name": true, "created_at": true, "total_supply": true, "fractions": true}[req.SortBy] {
		return fmt.Errorf("invalid sort_by")
	}
	if req.Order != "" && strings.ToLower(req.Order) != "asc" && strings.ToLower(req.Order) != "desc" {
		return fmt.Errorf("order must be asc or desc")
	}
	return nil
}

func (req *SearchRequest) normalizePagination() {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 || req.Limit > 100 {
		req.Limit = 10
	}
}

func (req *SearchRequest) assetTypes() []string {
	if len(req.AssetTypes) > 0 {
		return req.AssetTypes
	}
	if req.AssetType != "" {
		return []string{req.AssetType}
	}
	return nil
}
func (req *SearchRequest) locations() []string {
	if len(req.Locations) > 0 {
		return req.Locations
	}
	if req.Location != "" {
		return []string{req.Location}
	}
	return nil
}

// IndexAsset is a no-op for the DB backend
func (s *DBSearchBackend) IndexAsset(ctx context.Context, asset *models.Asset) error {
	return nil
}

// DeleteAssetFromIndex is a no-op for the DB backend
func (s *DBSearchBackend) DeleteAssetFromIndex(ctx context.Context, assetID uint) error {
	return nil
}

// ReindexAll is a no-op for the DB backend
func (s *DBSearchBackend) ReindexAll(ctx context.Context) error {
	return nil
}

// RecordAnalytics is a no-op for the DB backend
func (s *DBSearchBackend) RecordAnalytics(ctx context.Context, event *SearchAnalyticsEvent) error {
	return nil
}

// ---- ESSearchBackend (Elasticsearch adapter) --------------------------------

// AssetDocument represents how an asset is indexed in Elasticsearch
type AssetDocument struct {
	ID           uint              `json:"id"`
	Name         string            `json:"name"`
	Symbol       string            `json:"symbol"`
	Description  string            `json:"description"`
	AssetType    string            `json:"asset_type"`
	TotalSupply  int64             `json:"total_supply"`
	Fractions    uint64            `json:"fractions"`
	ContractID   string            `json:"contract_id"`
	OwnerAddress string            `json:"owner_address"`
	Metadata     map[string]string `json:"metadata"`
	Location     string            `json:"location"`
	CurrentPrice int64             `json:"current_price"`
	ImageURL     string            `json:"image_url"`
	DocumentURL  string            `json:"document_url"`
	Verified     bool              `json:"verified"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	SearchText   string            `json:"search_text"`
}

// ESSearchBackend integrates with Elasticsearch with transparent fallback to DB
type ESSearchBackend struct {
	client    *elasticsearch.Client
	dbBackend SearchBackend // fallback
	indexName string
}

// NewESSearchBackend creates an ESSearchBackend with real Elasticsearch integration
func NewESSearchBackend(esBaseURL string, db *gorm.DB) (SearchBackend, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{esBaseURL},
	}
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		// Fall back to DB-only if ES unavailable
		return &ESSearchBackend{
			client:    nil,
			dbBackend: NewDBSearchBackend(db),
			indexName: "assets",
		}, nil
	}

	// Verify ES connection
	res, err := client.Info(client.Info.WithContext(context.Background()))
	if err != nil || res.IsError() {
		// Fall back to DB if ES is down
		return &ESSearchBackend{
			client:    nil,
			dbBackend: NewDBSearchBackend(db),
			indexName: "assets",
		}, nil
	}

	es := &ESSearchBackend{
		client:    client,
		dbBackend: NewDBSearchBackend(db),
		indexName: "assets",
	}

	// Initialize Elasticsearch index with analyzers
	if err := es.initializeIndex(context.Background()); err != nil {
		// Log error but don't fail - will use DB backend
	}

	return es, nil
}

// initializeIndex creates the Elasticsearch index with custom analyzers
func (es *ESSearchBackend) initializeIndex(ctx context.Context) error {
	if es.client == nil {
		return nil
	}

	// Check if index exists
	res, _ := es.client.Indices.Exists([]string{es.indexName})
	if res != nil && !res.IsError() {
		return nil // Index already exists
	}

	indexMapping := map[string]interface{}{
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
				"id":            map[string]interface{}{"type": "keyword"},
				"name":          map[string]interface{}{"type": "text", "analyzer": "asset_analyzer", "fields": map[string]interface{}{"keyword": map[string]interface{}{"type": "keyword"}}},
				"symbol":        map[string]interface{}{"type": "keyword"},
				"description":   map[string]interface{}{"type": "text", "analyzer": "asset_analyzer"},
				"asset_type":    map[string]interface{}{"type": "keyword"},
				"total_supply":  map[string]interface{}{"type": "long"},
				"fractions":     map[string]interface{}{"type": "long"},
				"contract_id":   map[string]interface{}{"type": "keyword"},
				"owner_address": map[string]interface{}{"type": "keyword"},
				"metadata":      map[string]interface{}{"type": "object"},
				"location":      map[string]interface{}{"type": "keyword"},
				"current_price": map[string]interface{}{"type": "long"},
				"image_url":     map[string]interface{}{"type": "keyword"},
				"document_url":  map[string]interface{}{"type": "keyword"},
				"verified":      map[string]interface{}{"type": "boolean"},
				"created_at":    map[string]interface{}{"type": "date"},
				"updated_at":    map[string]interface{}{"type": "date"},
				"search_text":   map[string]interface{}{"type": "text", "analyzer": "asset_analyzer"},
			},
		},
	}

	body, _ := json.Marshal(indexMapping)
	res, err := es.client.Indices.Create(es.indexName, es.client.Indices.Create.WithBody(strings.NewReader(string(body))))
	if err != nil || res.IsError() {
		return fmt.Errorf("failed to create ES index: %w", err)
	}

	return nil
}

func (es *ESSearchBackend) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	// Listings are the price authority and custom metadata is stored as JSON in
	// PostgreSQL. Use the DB planner for combinations that ES cannot join safely.
	if req.MinPrice != nil || req.MaxPrice != nil || len(req.Metadata) > 0 {
		return es.dbBackend.Search(ctx, req)
	}
	req.normalizePagination()
	if es.client == nil {
		return es.dbBackend.Search(ctx, req)
	}

	startTime := time.Now()

	// Build ES query
	query := es.buildESQuery(req)
	body, _ := json.Marshal(query)

	// Execute search
	res, err := es.client.Search(
		es.client.Search.WithContext(ctx),
		es.client.Search.WithIndex(es.indexName),
		es.client.Search.WithBody(strings.NewReader(string(body))),
		es.client.Search.WithSize(req.Limit),
		es.client.Search.WithFrom((req.Page-1)*req.Limit),
	)

	if err != nil || res.IsError() {
		// Fall back to DB search on error
		return es.dbBackend.Search(ctx, req)
	}

	// Parse response
	var esResp map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		return es.dbBackend.Search(ctx, req)
	}

	// Extract hits and total
	hitsData := esResp["hits"].(map[string]interface{})
	totalObj := hitsData["total"].(map[string]interface{})
	total := int64(totalObj["value"].(float64))
	hits := hitsData["hits"].([]interface{})

	// Convert ES documents to Asset models
	assets := make([]models.Asset, 0, len(hits))
	for _, hit := range hits {
		hitObj := hit.(map[string]interface{})
		source := hitObj["_source"].(map[string]interface{})

		// Parse metadata
		var metadata string
		if m, ok := source["metadata"].(map[string]interface{}); ok {
			metadataJSON, _ := json.Marshal(m)
			metadata = string(metadataJSON)
		}

		asset := models.Asset{
			ID:           uint(source["id"].(float64)),
			Name:         source["name"].(string),
			Symbol:       source["symbol"].(string),
			Description:  source["description"].(string),
			AssetType:    source["asset_type"].(string),
			TotalSupply:  int64(source["total_supply"].(float64)),
			Fractions:    uint64(source["fractions"].(float64)),
			ContractID:   source["contract_id"].(string),
			OwnerAddress: source["owner_address"].(string),
			Metadata:     metadata,
			ImageURL:     source["image_url"].(string),
			DocumentURL:  source["document_url"].(string),
			Verified:     source["verified"].(bool),
		}
		assets = append(assets, asset)
	}

	facets := es.buildESFacets(ctx, req)
	duration := time.Since(startTime).Seconds() * 1000

	return &SearchResult{
		Total:  total,
		Page:   req.Page,
		Limit:  req.Limit,
		Assets: assets,
		Facets: facets,
		Took:   duration,
	}, nil
}

func (es *ESSearchBackend) Suggest(ctx context.Context, query string, limit int) (*SuggestResult, error) {
	if es.client == nil {
		return es.dbBackend.Suggest(ctx, query, limit)
	}

	if limit <= 0 || limit > 20 {
		limit = 10
	}

	term := strings.TrimSpace(query)
	if term == "" {
		return &SuggestResult{Suggestions: []string{}}, nil
	}

	// Build completion suggester query
	suggestQuery := map[string]interface{}{
		"suggest": map[string]interface{}{
			"asset-suggest": map[string]interface{}{
				"prefix": term,
				"completion": map[string]interface{}{
					"field":           "name.completion",
					"size":            limit,
					"skip_duplicates": true,
				},
			},
		},
	}

	body, _ := json.Marshal(suggestQuery)
	res, err := es.client.Search(
		es.client.Search.WithContext(ctx),
		es.client.Search.WithIndex(es.indexName),
		es.client.Search.WithBody(strings.NewReader(string(body))),
	)

	if err != nil || res.IsError() {
		// Fall back to DB
		return es.dbBackend.Suggest(ctx, query, limit)
	}

	var esResp map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		return es.dbBackend.Suggest(ctx, query, limit)
	}

	suggestions := []string{}
	if suggest, ok := esResp["suggest"].(map[string]interface{}); ok {
		if assetSuggest, ok := suggest["asset-suggest"].([]interface{}); ok && len(assetSuggest) > 0 {
			if suggestions_, ok := assetSuggest[0].(map[string]interface{})["options"].([]interface{}); ok {
				for _, option := range suggestions_ {
					if optionMap, ok := option.(map[string]interface{}); ok {
						if text, ok := optionMap["text"].(string); ok {
							suggestions = append(suggestions, text)
						}
					}
				}
			}
		}
	}

	return &SuggestResult{Suggestions: suggestions}, nil
}

func (es *ESSearchBackend) IndexAsset(ctx context.Context, asset *models.Asset) error {
	if es.client == nil {
		return nil
	}

	// Parse metadata
	var metadata map[string]string
	if err := json.Unmarshal([]byte(asset.Metadata), &metadata); err != nil {
		metadata = make(map[string]string)
	}

	searchText := fmt.Sprintf("%s %s %s", asset.Name, asset.Symbol, asset.Description)

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
		Location:     metadata["location"],
		ImageURL:     asset.ImageURL,
		DocumentURL:  asset.DocumentURL,
		Verified:     asset.Verified,
		CreatedAt:    asset.CreatedAt,
		UpdatedAt:    asset.UpdatedAt,
		SearchText:   searchText,
	}

	body, _ := json.Marshal(doc)
	docID := fmt.Sprintf("%d", asset.ID)

	res, err := es.client.Index(
		es.indexName,
		strings.NewReader(string(body)),
		es.client.Index.WithContext(ctx),
		es.client.Index.WithDocumentID(docID),
	)

	if err != nil || res.IsError() {
		return fmt.Errorf("failed to index asset: %w", err)
	}

	return nil
}

func (es *ESSearchBackend) DeleteAssetFromIndex(ctx context.Context, assetID uint) error {
	if es.client == nil {
		return nil
	}

	docID := fmt.Sprintf("%d", assetID)
	res, err := es.client.Delete(
		es.indexName,
		docID,
		es.client.Delete.WithContext(ctx),
	)

	if err != nil || res.IsError() {
		return fmt.Errorf("failed to delete asset from index: %w", err)
	}

	return nil
}

func (es *ESSearchBackend) ReindexAll(ctx context.Context) error {
	if es.client == nil {
		return nil
	}

	// Delete existing index
	es.client.Indices.Delete([]string{es.indexName})

	// Recreate index
	if err := es.initializeIndex(ctx); err != nil {
		return err
	}

	return nil
}

func (es *ESSearchBackend) RecordAnalytics(ctx context.Context, event *SearchAnalyticsEvent) error {
	if es.client == nil {
		return nil
	}

	analyticsIndex := "search_analytics"
	body, _ := json.Marshal(event)

	res, err := es.client.Index(
		analyticsIndex,
		strings.NewReader(string(body)),
		es.client.Index.WithContext(ctx),
	)

	if err != nil || res.IsError() {
		return fmt.Errorf("failed to record search analytics: %w", err)
	}

	return nil
}

func (es *ESSearchBackend) buildESQuery(req *SearchRequest) map[string]interface{} {
	filters := []map[string]interface{}{}

	// Full-text search on multiple fields with boosting
	if term := strings.TrimSpace(req.Query); term != "" {
		filters = append(filters, map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":     term,
				"fields":    []string{"name^3", "symbol^2", "description", "search_text"},
				"fuzziness": "AUTO",
			},
		})
	}

	// Asset type filter
	if types := req.assetTypes(); len(types) > 0 {
		filters = append(filters, map[string]interface{}{
			"terms": map[string]interface{}{
				"asset_type": types,
			},
		})
	}
	if locations := req.locations(); len(locations) > 0 {
		filters = append(filters, map[string]interface{}{"terms": map[string]interface{}{"location": locations}})
	}
	if req.CreatedFrom != nil || req.CreatedTo != nil {
		dateRange := map[string]interface{}{}
		if req.CreatedFrom != nil {
			dateRange["gte"] = req.CreatedFrom.UTC().Format(time.RFC3339)
		}
		if req.CreatedTo != nil {
			dateRange["lt"] = req.CreatedTo.UTC().Add(24 * time.Hour).Format(time.RFC3339)
		}
		filters = append(filters, map[string]interface{}{"range": map[string]interface{}{"created_at": dateRange}})
	}

	// Verified status filter
	if req.Verified != nil {
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{
				"verified": *req.Verified,
			},
		})
	}

	// Price range filter (would require join with listings in production)
	// Simplified for ES - in production, might denormalize current min/max price

	sort := []map[string]interface{}{}
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}

	switch sortBy {
	case "name":
		sort = append(sort, map[string]interface{}{"name.keyword": map[string]interface{}{"order": strings.ToLower(req.Order)}})
	case "created_at":
		sort = append(sort, map[string]interface{}{"created_at": map[string]interface{}{"order": strings.ToLower(req.Order)}})
	case "total_supply":
		sort = append(sort, map[string]interface{}{"total_supply": map[string]interface{}{"order": strings.ToLower(req.Order)}})
	case "fractions":
		sort = append(sort, map[string]interface{}{"fractions": map[string]interface{}{"order": strings.ToLower(req.Order)}})
	default:
		sort = append(sort, map[string]interface{}{"created_at": map[string]interface{}{"order": "desc"}})
	}

	query := map[string]interface{}{}

	if len(filters) == 0 {
		query["match_all"] = map[string]interface{}{}
	} else if len(filters) == 1 {
		query = filters[0]
	} else {
		query["bool"] = map[string]interface{}{
			"must": filters,
		}
	}

	return map[string]interface{}{
		"query": query,
		"sort":  sort,
	}
}

func (es *ESSearchBackend) buildESFacets(ctx context.Context, req *SearchRequest) SearchFacets {
	// Fall back to DB for facets to ensure accuracy
	if dbBackend, ok := es.dbBackend.(*DBSearchBackend); ok {
		return dbBackend.buildFacets(ctx, req)
	}
	return SearchFacets{}
}
