package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	searchconfig "github.com/yourusername/kor-assetforge/config"
	searchmetrics "github.com/yourusername/kor-assetforge/metrics"
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
	Categories  []string          `form:"category"`
	Tags        []string          `form:"tag"`
	CreatedFrom *time.Time        `form:"created_from" time_format:"2006-01-02"`
	CreatedTo   *time.Time        `form:"created_to" time_format:"2006-01-02"`
	Verified    *bool             `form:"verified"`
	Metadata    map[string]string `form:"-"`
	SortBy      string            `form:"sort_by"` // name | created_at | total_supply | fractions
	Order       string            `form:"order"`   // asc | desc
	Page        int               `form:"page"`
	Limit       int               `form:"limit"`
	UserID      uint              `form:"-"`
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
	RankingVariant   string         `json:"ranking_variant,omitempty"`
}

// SearchFacets carries aggregated filter counts for faceted navigation.
type SearchFacets struct {
	AssetTypes []FacetBucket `json:"asset_types"`
	Locations  []FacetBucket `json:"locations"`
	Verified   []FacetBucket `json:"verified"`
	Categories []FacetBucket `json:"categories"`
	Tags       []FacetBucket `json:"tags"`
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
	if len(req.Categories) > 0 {
		categoryAssets := s.db.Table("asset_categories").
			Select("asset_categories.asset_id").
			Joins("JOIN categories ON categories.id = asset_categories.category_id").
			Where("categories.slug IN ? AND categories.deleted_at IS NULL", cleanValues(req.Categories))
		q = q.Where("id IN (?)", categoryAssets)
	}
	if len(req.Tags) > 0 {
		tagAssets := s.db.Table("asset_tags").
			Select("asset_tags.asset_id").
			Joins("JOIN tags ON tags.id = asset_tags.tag_id").
			Where("tags.slug IN ? AND tags.deleted_at IS NULL", cleanValues(req.Tags))
		q = q.Where("id IN (?)", tagAssets)
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
	like := strings.ToLower(term) + "%"

	type row struct{ Name string }
	var names []row
	// LOWER + LIKE works in PostgreSQL and SQLite, which keeps the fallback and
	// its tests portable. A wider candidate set enables typo-tolerant ranking.
	if err := s.db.WithContext(ctx).Model(&models.Asset{}).
		Select("DISTINCT name").
		Where("LOWER(name) LIKE ? OR LOWER(symbol) LIKE ?", like, like).
		Limit(limit).Scan(&names).Error; err != nil {
		return nil, err
	}
	suggestions := make([]string, 0, limit)
	for _, r := range names {
		suggestions = append(suggestions, r.Name)
	}
	if len(suggestions) < limit {
		var candidates []row
		if err := s.db.WithContext(ctx).Model(&models.Asset{}).
			Select("DISTINCT name").Limit(200).Scan(&candidates).Error; err == nil {
			sort.SliceStable(candidates, func(i, j int) bool {
				return typoDistance(candidates[i].Name, term) < typoDistance(candidates[j].Name, term)
			})
			threshold := maxInt(2, len([]rune(term))/3)
			for _, candidate := range candidates {
				if typoDistance(candidate.Name, term) <= threshold {
					suggestions = append(suggestions, candidate.Name)
				}
			}
		}
	}
	suggestions = dedupeStrings(suggestions, limit)
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
	type namedBucket struct {
		Value string
		Count int64
	}
	var categoryBuckets, tagBuckets []namedBucket
	s.db.WithContext(ctx).Table("asset_categories").
		Select("categories.slug AS value, COUNT(DISTINCT asset_categories.asset_id) AS count").
		Joins("JOIN categories ON categories.id = asset_categories.category_id").
		Where("categories.deleted_at IS NULL").Group("categories.slug").Scan(&categoryBuckets)
	s.db.WithContext(ctx).Table("asset_tags").
		Select("tags.slug AS value, COUNT(DISTINCT asset_tags.asset_id) AS count").
		Joins("JOIN tags ON tags.id = asset_tags.tag_id").
		Where("tags.deleted_at IS NULL").Group("tags.slug").Scan(&tagBuckets)
	categoryFacets := make([]FacetBucket, 0, len(categoryBuckets))
	for _, bucket := range categoryBuckets {
		categoryFacets = append(categoryFacets, FacetBucket{Value: bucket.Value, Count: bucket.Count})
	}
	tagFacets := make([]FacetBucket, 0, len(tagBuckets))
	for _, bucket := range tagBuckets {
		tagFacets = append(tagFacets, FacetBucket{Value: bucket.Value, Count: bucket.Count})
	}

	_ = req
	return SearchFacets{
		AssetTypes: typeFacets,
		Locations:  locationFacets,
		Verified:   []FacetBucket{{Value: "true", Count: verifiedCount}, {Value: "false", Count: unverifiedCount}},
		Categories: categoryFacets,
		Tags:       tagFacets,
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
	if len(req.Categories) > 20 || len(req.Tags) > 20 {
		return fmt.Errorf("at most 20 categories or tags are allowed")
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

func cleanValues(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" && !seen[part] {
				seen[part] = true
				result = append(result, part)
			}
		}
	}
	return result
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
	ID              uint              `json:"id"`
	Name            string            `json:"name"`
	Symbol          string            `json:"symbol"`
	Description     string            `json:"description"`
	AssetType       string            `json:"asset_type"`
	TotalSupply     int64             `json:"total_supply"`
	Fractions       uint64            `json:"fractions"`
	ContractID      string            `json:"contract_id"`
	OwnerAddress    string            `json:"owner_address"`
	Metadata        map[string]string `json:"metadata"`
	Location        string            `json:"location"`
	CurrentPrice    int64             `json:"current_price"`
	ImageURL        string            `json:"image_url"`
	DocumentURL     string            `json:"document_url"`
	Verified        bool              `json:"verified"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	SearchText      string            `json:"search_text"`
	Categories      []string          `json:"categories,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	PopularityScore float64           `json:"popularity_score"`
}

// ESSearchBackend integrates with Elasticsearch with transparent fallback to DB
type ESSearchBackend struct {
	client        *elasticsearch.Client
	dbBackend     SearchBackend // fallback
	indexName     string
	stateMu       sync.RWMutex
	esAvailable   bool
	lastProbe     time.Time
	probeInterval time.Duration
	ranker        *SearchRanker
}

// NewESSearchBackend creates an ESSearchBackend with real Elasticsearch integration
func NewESSearchBackend(esBaseURL string, db *gorm.DB) (SearchBackend, error) {
	es := &ESSearchBackend{
		dbBackend:     NewDBSearchBackend(db),
		indexName:     "assets",
		probeInterval: 15 * time.Second,
		ranker:        NewSearchRanker(db, searchconfig.LoadSearchConfig()),
	}
	if strings.TrimSpace(esBaseURL) == "" {
		searchmetrics.ElasticsearchAvailable.Set(0)
		return es, nil
	}
	cfg := elasticsearch.Config{
		Addresses: []string{esBaseURL},
	}
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		searchmetrics.ElasticsearchAvailable.Set(0)
		return es, nil
	}
	es.client = client

	// Verify ES connection
	res, err := client.Info(client.Info.WithContext(context.Background()))
	if err != nil || res.IsError() {
		if res != nil {
			res.Body.Close()
		}
		es.lastProbe = time.Now()
		searchmetrics.ElasticsearchAvailable.Set(0)
		log.Printf("search: Elasticsearch unavailable at startup; database fallback enabled")
		return es, nil
	}
	res.Body.Close()
	es.esAvailable = true
	searchmetrics.ElasticsearchAvailable.Set(1)

	// Initialize Elasticsearch index with analyzers
	if err := es.initializeIndex(context.Background()); err != nil {
		// Log error but don't fail - will use DB backend
	}

	return es, nil
}

// elasticsearchReady probes a degraded connection periodically. Keeping the
// client while degraded lets normal traffic recover automatically without a
// process restart once Elasticsearch becomes healthy again.
func (es *ESSearchBackend) elasticsearchReady(ctx context.Context) bool {
	if es.client == nil {
		return false
	}
	es.stateMu.RLock()
	available, lastProbe := es.esAvailable, es.lastProbe
	es.stateMu.RUnlock()
	if available {
		return true
	}
	if time.Since(lastProbe) < es.probeInterval {
		return false
	}

	es.stateMu.Lock()
	if es.esAvailable {
		es.stateMu.Unlock()
		return true
	}
	if time.Since(es.lastProbe) < es.probeInterval {
		es.stateMu.Unlock()
		return false
	}
	es.lastProbe = time.Now()
	es.stateMu.Unlock()

	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	res, err := es.client.Info(es.client.Info.WithContext(probeCtx))
	if err != nil || res.IsError() {
		if res != nil {
			res.Body.Close()
		}
		return false
	}
	res.Body.Close()
	es.stateMu.Lock()
	es.esAvailable = true
	es.stateMu.Unlock()
	searchmetrics.ElasticsearchAvailable.Set(1)
	searchmetrics.ElasticsearchRecoveries.Inc()
	log.Printf("search: Elasticsearch connection recovered; ending database fallback")
	return true
}

func (es *ESSearchBackend) markUnavailable(reason string) {
	es.stateMu.Lock()
	wasAvailable := es.esAvailable
	es.esAvailable = false
	es.lastProbe = time.Now()
	es.stateMu.Unlock()
	searchmetrics.ElasticsearchAvailable.Set(0)
	if wasAvailable {
		log.Printf("search: Elasticsearch became unavailable (%s); database fallback enabled", reason)
	}
}

func (es *ESSearchBackend) fallback(ctx context.Context, operation, reason string, fn func() (interface{}, error)) (interface{}, error) {
	searchmetrics.SearchFallbacks.WithLabelValues(operation, reason).Inc()
	searchmetrics.SearchBackendRequests.WithLabelValues(operation, "database").Inc()
	start := time.Now()
	result, err := fn()
	searchmetrics.SearchBackendDuration.WithLabelValues(operation, "database").Observe(time.Since(start).Seconds())
	if err != nil {
		log.Printf("search: database fallback failed operation=%s reason=%s error=%v", operation, reason, err)
	}
	return result, err
}

// initializeIndex creates the Elasticsearch index with custom analyzers
func (es *ESSearchBackend) initializeIndex(ctx context.Context) error {
	if es.client == nil {
		return nil
	}

	// Check if index exists
	res, _ := es.client.Indices.Exists([]string{es.indexName})
	if res != nil && !res.IsError() {
		res.Body.Close()
		return nil // Index already exists
	}
	if res != nil {
		res.Body.Close()
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
				"id":               map[string]interface{}{"type": "keyword"},
				"name":             map[string]interface{}{"type": "text", "analyzer": "asset_analyzer", "fields": map[string]interface{}{"keyword": map[string]interface{}{"type": "keyword"}}},
				"symbol":           map[string]interface{}{"type": "keyword"},
				"description":      map[string]interface{}{"type": "text", "analyzer": "asset_analyzer"},
				"asset_type":       map[string]interface{}{"type": "keyword"},
				"total_supply":     map[string]interface{}{"type": "long"},
				"fractions":        map[string]interface{}{"type": "long"},
				"contract_id":      map[string]interface{}{"type": "keyword"},
				"owner_address":    map[string]interface{}{"type": "keyword"},
				"metadata":         map[string]interface{}{"type": "object"},
				"location":         map[string]interface{}{"type": "keyword"},
				"current_price":    map[string]interface{}{"type": "long"},
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

	body, _ := json.Marshal(indexMapping)
	res, err := es.client.Indices.Create(es.indexName, es.client.Indices.Create.WithBody(strings.NewReader(string(body))))
	if err != nil || res.IsError() {
		if res != nil {
			res.Body.Close()
		}
		return fmt.Errorf("failed to create ES index: %w", err)
	}
	res.Body.Close()

	return nil
}

func (es *ESSearchBackend) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	// Listings are the price authority and custom metadata is stored as JSON in
	// PostgreSQL. Use the DB planner for combinations that ES cannot join safely.
	if req.MinPrice != nil || req.MaxPrice != nil || len(req.Metadata) > 0 {
		start := time.Now()
		result, err := es.dbBackend.Search(ctx, req)
		searchmetrics.SearchBackendRequests.WithLabelValues("search", "database").Inc()
		searchmetrics.SearchBackendDuration.WithLabelValues("search", "database").Observe(time.Since(start).Seconds())
		return result, err
	}
	req.normalizePagination()
	if !es.elasticsearchReady(ctx) {
		result, err := es.fallback(ctx, "search", "unavailable", func() (interface{}, error) {
			return es.dbBackend.Search(ctx, req)
		})
		if result == nil {
			return nil, err
		}
		return result.(*SearchResult), err
	}

	startTime := time.Now()

	// Build ES query
	query, rankingVariant := es.buildESQueryContext(ctx, req)
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
		if res != nil {
			res.Body.Close()
		}
		es.markUnavailable("search request failed")
		result, fallbackErr := es.fallback(ctx, "search", "request_error", func() (interface{}, error) {
			return es.dbBackend.Search(ctx, req)
		})
		if result == nil {
			return nil, fallbackErr
		}
		return result.(*SearchResult), fallbackErr
	}
	defer res.Body.Close()

	// Parse response
	var esResp struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source AssetDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		result, fallbackErr := es.fallback(ctx, "search", "invalid_response", func() (interface{}, error) {
			return es.dbBackend.Search(ctx, req)
		})
		if result == nil {
			return nil, fallbackErr
		}
		return result.(*SearchResult), fallbackErr
	}

	// Convert ES documents to Asset models
	assets := make([]models.Asset, 0, len(esResp.Hits.Hits))
	for _, hit := range esResp.Hits.Hits {
		source := hit.Source
		metadataJSON, _ := json.Marshal(source.Metadata)
		asset := models.Asset{
			ID: source.ID, Name: source.Name, Symbol: source.Symbol,
			Description: source.Description, AssetType: source.AssetType,
			TotalSupply: source.TotalSupply, Fractions: source.Fractions,
			ContractID: source.ContractID, OwnerAddress: source.OwnerAddress,
			Metadata: string(metadataJSON), ImageURL: source.ImageURL,
			DocumentURL: source.DocumentURL, Verified: source.Verified,
			CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		}
		assets = append(assets, asset)
	}

	facets := es.buildESFacets(ctx, req)
	duration := time.Since(startTime).Seconds() * 1000
	searchmetrics.SearchBackendRequests.WithLabelValues("search", "elasticsearch").Inc()
	searchmetrics.SearchBackendDuration.WithLabelValues("search", "elasticsearch").Observe(duration / 1000)
	quality := 0.0
	if esResp.Hits.Total.Value > 0 {
		quality = 1
	}
	searchmetrics.SearchResultQuality.WithLabelValues("non_zero_results", rankingVariant).Observe(quality)

	return &SearchResult{
		Total:          esResp.Hits.Total.Value,
		Page:           req.Page,
		Limit:          req.Limit,
		Assets:         assets,
		Facets:         facets,
		Took:           duration,
		RankingVariant: rankingVariant,
	}, nil
}

func (es *ESSearchBackend) Suggest(ctx context.Context, query string, limit int) (*SuggestResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	term := strings.TrimSpace(query)
	if term == "" {
		return &SuggestResult{Suggestions: []string{}}, nil
	}
	if !es.elasticsearchReady(ctx) {
		result, err := es.fallback(ctx, "suggest", "unavailable", func() (interface{}, error) {
			return es.dbBackend.Suggest(ctx, query, limit)
		})
		if result == nil {
			return nil, err
		}
		return result.(*SuggestResult), err
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
					"fuzzy":           map[string]interface{}{"fuzziness": "AUTO"},
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
		if res != nil {
			res.Body.Close()
		}
		es.markUnavailable("suggest request failed")
		result, fallbackErr := es.fallback(ctx, "suggest", "request_error", func() (interface{}, error) {
			return es.dbBackend.Suggest(ctx, query, limit)
		})
		if result == nil {
			return nil, fallbackErr
		}
		return result.(*SuggestResult), fallbackErr
	}
	defer res.Body.Close()

	var esResp map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		result, fallbackErr := es.fallback(ctx, "suggest", "invalid_response", func() (interface{}, error) {
			return es.dbBackend.Suggest(ctx, query, limit)
		})
		if result == nil {
			return nil, fallbackErr
		}
		return result.(*SuggestResult), fallbackErr
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

	searchmetrics.SearchBackendRequests.WithLabelValues("suggest", "elasticsearch").Inc()
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
	if dbBackend, ok := es.dbBackend.(*DBSearchBackend); ok && dbBackend.db != nil {
		dbBackend.db.Model(&models.Category{}).
			Joins("JOIN asset_categories ON asset_categories.category_id = categories.id").
			Where("asset_categories.asset_id = ?", asset.ID).Pluck("categories.slug", &doc.Categories)
		dbBackend.db.Model(&models.Tag{}).
			Joins("JOIN asset_tags ON asset_tags.tag_id = tags.id").
			Where("asset_tags.asset_id = ?", asset.ID).Pluck("tags.slug", &doc.Tags)
		var interactions int64
		dbBackend.db.Model(&models.UserInteraction{}).Where("asset_id = ?", asset.ID).Count(&interactions)
		doc.PopularityScore = float64(interactions)
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
	query, _ := es.buildESQueryContext(context.Background(), req)
	return query
}

func (es *ESSearchBackend) buildESQueryContext(ctx context.Context, req *SearchRequest) (map[string]interface{}, string) {
	filters := []map[string]interface{}{}
	rankingVariant := RankingControl

	// Full-text search on multiple fields with boosting
	if term := strings.TrimSpace(req.Query); term != "" {
		ranker := es.ranker
		if ranker == nil {
			ranker = NewSearchRanker(nil, searchconfig.LoadSearchConfig())
		}
		ranked, variant := ranker.RankedQuery(ctx, req)
		filters = append(filters, ranked)
		rankingVariant = variant
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
	if categories := cleanValues(req.Categories); len(categories) > 0 {
		filters = append(filters, map[string]interface{}{"terms": map[string]interface{}{"categories.keyword": categories}})
	}
	if tags := cleanValues(req.Tags); len(tags) > 0 {
		filters = append(filters, map[string]interface{}{"terms": map[string]interface{}{"tags.keyword": tags}})
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
	if sortBy == "" && strings.TrimSpace(req.Query) != "" {
		sortBy = "relevance"
	} else if sortBy == "" {
		sortBy = "created_at"
	}

	switch sortBy {
	case "relevance":
		sort = append(sort, map[string]interface{}{"_score": map[string]interface{}{"order": "desc"}})
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
	}, rankingVariant
}

func (es *ESSearchBackend) buildESFacets(ctx context.Context, req *SearchRequest) SearchFacets {
	// Fall back to DB for facets to ensure accuracy
	if dbBackend, ok := es.dbBackend.(*DBSearchBackend); ok {
		return dbBackend.buildFacets(ctx, req)
	}
	return SearchFacets{}
}
