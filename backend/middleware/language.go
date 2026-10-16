package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/yourusername/kor-assetforge/i18n"
)

// LanguageContextKey is the gin context key the resolved language is stored under.
const LanguageContextKey = "language"

// DetectLanguage resolves the request's preferred language from, in order of
// precedence, the "lang" query parameter, the "X-Language" header, and the
// standard Accept-Language header. It falls back to i18n.DefaultLanguage if
// none of those name a supported language, and stores the result on the gin
// context under LanguageContextKey for handlers to read.
func DetectLanguage() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := resolveLanguage(c)
		c.Set(LanguageContextKey, lang)
		c.Header("Content-Language", lang)
		c.Next()
	}
}

// LanguageFromContext returns the language resolved by DetectLanguage, or the
// default language if the middleware was not run.
func LanguageFromContext(c *gin.Context) string {
	if lang, ok := c.Get(LanguageContextKey); ok {
		if s, ok := lang.(string); ok {
			return s
		}
	}
	return i18n.DefaultLanguage
}

func resolveLanguage(c *gin.Context) string {
	if q := c.Query("lang"); q != "" && i18n.Default().IsSupported(q) {
		return normalizeSupported(q)
	}

	if h := c.GetHeader("X-Language"); h != "" && i18n.Default().IsSupported(h) {
		return normalizeSupported(h)
	}

	if accept := c.GetHeader("Accept-Language"); accept != "" {
		if lang := parseAcceptLanguage(accept); lang != "" {
			return lang
		}
	}

	return i18n.DefaultLanguage
}

// parseAcceptLanguage picks the highest-weighted supported language from a
// standard Accept-Language header, e.g. "fr-CA,fr;q=0.9,en;q=0.8".
func parseAcceptLanguage(header string) string {
	type weighted struct {
		lang   string
		weight float64
	}

	var candidates []weighted
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments := strings.Split(part, ";")
		lang := strings.TrimSpace(segments[0])
		weight := 1.0
		for _, seg := range segments[1:] {
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "q=") {
				if w, err := strconv.ParseFloat(strings.TrimPrefix(seg, "q="), 64); err == nil {
					weight = w
				}
			}
		}
		candidates = append(candidates, weighted{lang: lang, weight: weight})
	}

	best := ""
	bestWeight := -1.0
	for _, cand := range candidates {
		if !i18n.Default().IsSupported(cand.lang) {
			continue
		}
		if cand.weight > bestWeight {
			bestWeight = cand.weight
			best = cand.lang
		}
	}
	return normalizeSupported(best)
}

func normalizeSupported(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i != -1 {
		lang = lang[:i]
	}
	return lang
}
