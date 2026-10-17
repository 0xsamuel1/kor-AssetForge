package utils

import "github.com/microcosm-cc/bluemonday"

var userContentPolicy = func() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()
	policy.AllowElements("p", "br", "strong", "em", "ul", "ol", "li", "blockquote", "code", "pre")
	policy.AllowAttrs("href").OnElements("a")
	policy.RequireNoFollowOnLinks(true)
	policy.AllowURLSchemes("http", "https", "mailto")
	return policy
}()

// SanitizeUserContent removes unsafe markup while preserving a small,
// documented set of formatting elements for user-generated content.
func SanitizeUserContent(value string) string {
	return userContentPolicy.Sanitize(value)
}
