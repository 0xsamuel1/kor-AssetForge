// Package i18n provides translation lookup for API error messages, email
// templates, and other user-facing content across the supported locales.
package i18n

import (
	"embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localeFiles embed.FS

// DefaultLanguage is used whenever a request does not specify a supported language.
const DefaultLanguage = "en"

// SupportedLanguages lists the language codes the platform currently ships translations for.
var SupportedLanguages = []string{"en", "es", "fr", "zh"}

// Service resolves translation keys to localized strings.
type Service struct {
	mu     sync.RWMutex
	tables map[string]map[string]string
}

var (
	defaultService     *Service
	defaultServiceOnce sync.Once
)

// New loads all locale files embedded in backend/locales and returns a ready-to-use Service.
func New() (*Service, error) {
	s := &Service{tables: make(map[string]map[string]string)}
	for _, lang := range SupportedLanguages {
		data, err := localeFiles.ReadFile("locales/" + lang + ".json")
		if err != nil {
			return nil, err
		}
		var table map[string]string
		if err := json.Unmarshal(data, &table); err != nil {
			return nil, err
		}
		s.tables[lang] = table
	}
	return s, nil
}

// Default returns a process-wide Service instance, initializing it on first use.
// Since translations are embedded at compile time, loading cannot fail at runtime.
func Default() *Service {
	defaultServiceOnce.Do(func() {
		svc, err := New()
		if err != nil {
			panic("i18n: failed to load embedded locale files: " + err.Error())
		}
		defaultService = svc
	})
	return defaultService
}

// IsSupported reports whether the given language code has a translation table.
func (s *Service) IsSupported(lang string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.tables[normalizeLang(lang)]
	return ok
}

// Translate returns the localized string for key in lang, substituting any
// {{placeholder}} tokens with values from vars. Falls back to DefaultLanguage,
// then to the key itself, if no translation is found.
func (s *Service) Translate(lang, key string, vars map[string]string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lang = normalizeLang(lang)
	value, ok := s.tables[lang][key]
	if !ok {
		value, ok = s.tables[DefaultLanguage][key]
	}
	if !ok {
		return key
	}

	for k, v := range vars {
		value = strings.ReplaceAll(value, "{{"+k+"}}", v)
	}
	return value
}

// T is a package-level convenience wrapper around Default().Translate.
func T(lang, key string, vars map[string]string) string {
	return Default().Translate(lang, key, vars)
}

func normalizeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i != -1 {
		lang = lang[:i]
	}
	for _, supported := range SupportedLanguages {
		if lang == supported {
			return supported
		}
	}
	return DefaultLanguage
}
