package utils

import (
	"strings"
	"testing"
)

func TestSanitizeUserContentRemovesUnsafeMarkup(t *testing.T) {
	got := SanitizeUserContent(`<p>safe</p><script>alert(1)</script><a href="javascript:alert(1)">bad</a>`)
	if strings.Contains(got, "<script") || strings.Contains(got, "javascript:") {
		t.Fatalf("unsafe markup survived sanitization: %q", got)
	}
	if !strings.Contains(got, "<p>safe</p>") {
		t.Fatalf("allowed markup was removed: %q", got)
	}
}
