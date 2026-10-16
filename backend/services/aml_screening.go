package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/kor-assetforge/config"
)

// AMLScreeningProvider is the interface for an automated sanctions/PEP/
// adverse-media screening call. It is intentionally separate from
// KYCProvider.ScreenAML: KYCProvider models one bundled third-party identity
// vendor, whereas AML screening is commonly a distinct, swappable vendor
// (ComplyAdvantage, Refinitiv World-Check, LexisNexis, etc.) screened against
// named watchlists -- see #377.
type AMLScreeningProvider interface {
	// Screen runs a sanctions/PEP screen for the given individual against
	// the configured watchlists and returns the result. lists is the set
	// of watchlists actually screened (e.g. ["OFAC", "UN", "EU"]).
	Screen(fullName, nationality, dateOfBirth string) (*AMLResult, error)
	// Lists reports which sanctions lists this provider screens against.
	Lists() []string
}

// NewAMLScreeningProvider builds the configured AML provider. An unrecognized
// or empty cfg.Provider returns a NoopAMLProvider -- screening is then never
// silently treated as "clear"; every call explicitly fails until a real
// provider is configured, because an unscreened user must never be
// mistaken for a cleared one.
func NewAMLScreeningProvider(cfg config.ComplianceConfig) AMLScreeningProvider {
	switch cfg.Provider {
	case "complyadvantage":
		return NewComplyAdvantageProvider(cfg)
	default:
		return NewNoopAMLProvider(cfg.SanctionsLists)
	}
}

// ---- ComplyAdvantage -------------------------------------------------------

const complyAdvantageDefaultBaseURL = "https://api.complyadvantage.com"

// ComplyAdvantageProvider screens against ComplyAdvantage's Search API.
// ComplyAdvantage does not publish an official Go SDK (only REST, and
// SDKs for a handful of other languages), so this calls the documented
// REST endpoint directly rather than depending on a fictitious package.
//
// NOTE: the request/response shape below is written from ComplyAdvantage's
// public Search API documentation. Validate field names and the exact
// fuzziness/filter semantics against the current API docs and a sandbox
// API key before relying on this in production -- the vendor's contract
// can change, and this has not been exercised against the live service.
type ComplyAdvantageProvider struct {
	apiKey  string
	baseURL string
	lists   []string
	client  *http.Client
}

// NewComplyAdvantageProvider constructs a ComplyAdvantageProvider from config.
func NewComplyAdvantageProvider(cfg config.ComplianceConfig) *ComplyAdvantageProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = complyAdvantageDefaultBaseURL
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ComplyAdvantageProvider{
		apiKey:  cfg.APIKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		lists:   cfg.SanctionsLists,
		client:  &http.Client{Timeout: timeout},
	}
}

func (p *ComplyAdvantageProvider) Lists() []string { return p.lists }

type complyAdvantageSearchRequest struct {
	SearchTerm string                      `json:"search_term"`
	Fuzziness  float64                     `json:"fuzziness"`
	Filters    complyAdvantageSearchFilter `json:"filters"`
}

type complyAdvantageSearchFilter struct {
	Types          []string `json:"types"`                     // e.g. ["sanction", "warning", "pep"]
	BirthYear      int      `json:"birth_year,omitempty"`
	CountryCodes   []string `json:"country_codes,omitempty"`
	ExactMatch     bool     `json:"exact_match"`
}

type complyAdvantageSearchResponse struct {
	Content struct {
		Data struct {
			SearchID string `json:"id"`
			Hits     []struct {
				MatchStatus string  `json:"match_status"` // "potential_match" | "false_positive" | "true_positive" | "no_match"
				Score       float64 `json:"score"`
				Doc         struct {
					Name  string   `json:"name"`
					Types []string `json:"types"`
				} `json:"doc"`
			} `json:"hits"`
		} `json:"data"`
	} `json:"content"`
}

func (p *ComplyAdvantageProvider) Screen(fullName, nationality, dateOfBirth string) (*AMLResult, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("AML_PROVIDER_API_KEY is not configured for provider=complyadvantage")
	}
	if fullName == "" {
		return nil, fmt.Errorf("fullName is required for AML screening")
	}

	reqBody := complyAdvantageSearchRequest{
		SearchTerm: fullName,
		Fuzziness:  0.6,
		Filters: complyAdvantageSearchFilter{
			Types:      []string{"sanction", "warning", "pep"},
			ExactMatch: false,
		},
	}
	if nationality != "" {
		reqBody.Filters.CountryCodes = []string{nationality}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to encode AML screening request: %w", err)
	}

	url := fmt.Sprintf("%s/searches?api_key=%s", p.baseURL, p.apiKey)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build AML screening request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("AML provider request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read AML provider response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("AML provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed complyAdvantageSearchResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode AML provider response: %w", err)
	}

	matches := make([]string, 0, len(parsed.Content.Data.Hits))
	riskLevel := "low"
	status := "clear"
	for _, hit := range parsed.Content.Data.Hits {
		if hit.MatchStatus == "false_positive" || hit.MatchStatus == "no_match" {
			continue
		}
		matches = append(matches, fmt.Sprintf("%s (%s, score=%.2f)", hit.Doc.Name, strings.Join(hit.Doc.Types, "/"), hit.Score))
	}
	if len(matches) > 0 {
		status = "flagged"
		riskLevel = "high"
	}

	screeningID := parsed.Content.Data.SearchID
	if screeningID == "" {
		screeningID = uuid.NewString()
	}

	return &AMLResult{
		ScreeningID: screeningID,
		Status:      status,
		RiskLevel:   riskLevel,
		Matches:     matches,
		ScreenedAt:  time.Now(),
	}, nil
}

// ---- Noop (no provider configured) -----------------------------------------

// NoopAMLProvider is used when no real AML provider is configured. It never
// returns "clear" -- every screen comes back as a required manual review, so
// that an unconfigured environment cannot be mistaken for a cleared one.
// This is deliberately different from MockKYCProvider.ScreenAML (which
// simulates a working provider for local development of the KYC flow); this
// one is the honest "we are not actually screening" state.
type NoopAMLProvider struct {
	lists []string
}

// NewNoopAMLProvider returns a ready-to-use NoopAMLProvider.
func NewNoopAMLProvider(lists []string) *NoopAMLProvider {
	return &NoopAMLProvider{lists: lists}
}

func (p *NoopAMLProvider) Lists() []string { return p.lists }

func (p *NoopAMLProvider) Screen(fullName, nationality, dateOfBirth string) (*AMLResult, error) {
	if fullName == "" {
		return nil, fmt.Errorf("fullName is required for AML screening")
	}
	return &AMLResult{
		ScreeningID: uuid.NewString(),
		Status:      "flagged",
		RiskLevel:   "high",
		Matches:     []string{"no AML provider configured (AML_PROVIDER unset) -- routed to manual review"},
		ScreenedAt:  time.Now(),
	}, nil
}
