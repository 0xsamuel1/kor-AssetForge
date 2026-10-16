package services

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// AutocompleteResult gives clients enough context to distinguish direct
// completions, spelling corrections, personal history, and global trends.
type AutocompleteResult struct {
	Suggestions []string `json:"suggestions"`
	DidYouMean  string   `json:"did_you_mean,omitempty"`
	Recent      []string `json:"recent,omitempty"`
	Popular     []string `json:"popular,omitempty"`
}

// SearchSuggestionService enriches backend prefix/fuzzy candidates with
// bounded in-memory recent and popular search signals. These signals contain
// query text only and can be replaced by a shared store without API changes.
type SearchSuggestionService struct {
	backend SearchBackend
	mu      sync.RWMutex
	recent  map[uint][]string
	popular map[string]int
}

func NewSearchSuggestionService(backend SearchBackend) *SearchSuggestionService {
	return &SearchSuggestionService{
		backend: backend, recent: make(map[uint][]string), popular: make(map[string]int),
	}
}

func (s *SearchSuggestionService) RecordSearch(userID uint, query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.popular[strings.ToLower(query)]++
	if userID == 0 {
		return
	}
	items := []string{query}
	for _, previous := range s.recent[userID] {
		if !strings.EqualFold(previous, query) && len(items) < 10 {
			items = append(items, previous)
		}
	}
	s.recent[userID] = items
}

func (s *SearchSuggestionService) Autocomplete(ctx context.Context, userID uint, query string, limit int) (*AutocompleteResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	query = strings.TrimSpace(query)
	result := &AutocompleteResult{Suggestions: []string{}}
	if query != "" {
		backendResult, err := s.backend.Suggest(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		result.Suggestions = dedupeStrings(backendResult.Suggestions, limit)
		for _, candidate := range result.Suggestions {
			if !strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(query)) &&
				typoDistance(candidate, query) <= maxInt(2, len([]rune(query))/3) {
				result.DidYouMean = candidate
				break
			}
		}
	}

	s.mu.RLock()
	if userID != 0 {
		result.Recent = matchingStrings(s.recent[userID], query, 5)
	}
	type popularQuery struct {
		query string
		count int
	}
	popular := make([]popularQuery, 0, len(s.popular))
	for item, count := range s.popular {
		if query == "" || strings.Contains(item, strings.ToLower(query)) {
			popular = append(popular, popularQuery{item, count})
		}
	}
	s.mu.RUnlock()
	sort.Slice(popular, func(i, j int) bool {
		if popular[i].count == popular[j].count {
			return popular[i].query < popular[j].query
		}
		return popular[i].count > popular[j].count
	})
	for _, item := range popular {
		if len(result.Popular) == 5 {
			break
		}
		result.Popular = append(result.Popular, item.query)
	}
	return result, nil
}

func dedupeStrings(values []string, limit int) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func matchingStrings(values []string, query string, limit int) []string {
	result := make([]string, 0, limit)
	for _, value := range values {
		if query == "" || strings.Contains(strings.ToLower(value), strings.ToLower(query)) {
			result = append(result, value)
		}
		if len(result) == limit {
			break
		}
	}
	return result
}

func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	previous := make([]int, len(br)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, ca := range ar {
		current := make([]int, len(br)+1)
		current[0] = i + 1
		for j, cb := range br {
			cost := 0
			if ca != cb {
				cost = 1
			}
			current[j+1] = minInt(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(br)]
}

func typoDistance(candidate, query string) int {
	candidateRunes, queryRunes := []rune(strings.ToLower(candidate)), []rune(strings.ToLower(query))
	prefixLength := len(queryRunes)
	if len(candidateRunes) < prefixLength {
		prefixLength = len(candidateRunes)
	}
	return minInt(
		levenshtein(string(candidateRunes), string(queryRunes)),
		levenshtein(string(candidateRunes[:prefixLength]), string(queryRunes)),
	)
}

func minInt(values ...int) int {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
