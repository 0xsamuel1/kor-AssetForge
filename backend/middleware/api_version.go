package middleware

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	CurrentAPIVersion    = "v2"
	DeprecatedAPIVersion = "v1"

	headerAPIVersion  = "X-API-Version"
	headerDeprecation = "Deprecation"
	headerSunset      = "Sunset"
	headerLink        = "Link"
	headerVary        = "Vary"

	// SunsetDate is the planned end-of-life date for v1
	SunsetDate = "Sat, 31 Dec 2026 23:59:59 GMT"
)

var versionPattern = regexp.MustCompile(`^v[0-9]+$`)

// VersionFromPath extracts the version segment (e.g. "v1", "v2") from the
// URL path and stores it in the gin context under the key "api_version".
func VersionFromPath() gin.HandlerFunc {
	return func(c *gin.Context) {
		version := versionFromPath(c.Request.URL.Path)
		if version == "" {
			version = requestedVersion(c)
		}
		if version == "" {
			version = CurrentAPIVersion
		}
		c.Set("api_version", version)
		c.Header(headerAPIVersion, version)
		c.Header(headerVary, "Accept, X-API-Version")
		c.Next()
	}
}

func VersionNegotiation() gin.HandlerFunc {
	return func(c *gin.Context) {
		version := requestedVersion(c)
		pathVersion := versionFromPath(c.Request.URL.Path)
		if pathVersion != "" && version != "" && pathVersion != version {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error":             "conflicting API versions",
				"path_version":      pathVersion,
				"requested_version": version,
			})
			return
		}
		c.Next()
	}
}

// DeprecationWarning injects Deprecation / Sunset headers on all v1 responses
// so clients are notified to migrate.
func DeprecationWarning() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		version, _ := c.Get("api_version")
		if version == DeprecatedAPIVersion {
			c.Header(headerAPIVersion, DeprecatedAPIVersion)
			c.Header(headerDeprecation, "true")
			c.Header(headerSunset, SunsetDate)
			c.Header(headerLink, `</api/v2>; rel="successor-version", </api/version/migration-guide>; rel="deprecation"`)
		} else {
			c.Header(headerAPIVersion, CurrentAPIVersion)
		}
	}
}

func MigrationGuide(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"current_version":    CurrentAPIVersion,
		"deprecated_version": DeprecatedAPIVersion,
		"sunset":             SunsetDate,
		"breaking_changes": []string{
			"v2 responses use consistent data/error envelopes with api_version metadata.",
			"v2 list endpoints standardize pagination fields as page, page_size, and total_count.",
			"v1 endpoints return Deprecation and Sunset headers until the sunset date.",
		},
		"migration_steps": []string{
			"Move request paths from /api/v1 to /api/v2.",
			"Send X-API-Version: v2 or Accept: application/vnd.assetforge.v2+json during transition.",
			"Update response parsing to read nested data envelopes where present.",
		},
	})
}

// RequireMinVersion aborts with 410 Gone if the request targets a version
// older than the minimum supported version.
func RequireMinVersion(minVersion string) gin.HandlerFunc {
	return func(c *gin.Context) {
		version, exists := c.Get("api_version")
		if exists && versionOlderThan(version.(string), minVersion) {
			c.AbortWithStatusJSON(http.StatusGone, gin.H{
				"error":   "API version no longer supported",
				"version": version,
				"migrate": "/api/" + minVersion,
			})
			return
		}
		c.Next()
	}
}

// versionOlderThan compares simple "vN" version strings.
func versionOlderThan(v, min string) bool {
	return strings.TrimPrefix(v, "v") < strings.TrimPrefix(min, "v")
}

func versionFromPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, p := range parts {
		if versionPattern.MatchString(p) {
			return p
		}
	}
	return ""
}

func requestedVersion(c *gin.Context) string {
	if version := strings.TrimSpace(c.GetHeader(headerAPIVersion)); versionPattern.MatchString(version) {
		return version
	}
	accept := c.GetHeader("Accept")
	for _, part := range strings.Split(accept, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "version=") {
			version := strings.TrimPrefix(part, "version=")
			if versionPattern.MatchString(version) {
				return version
			}
		}
	}
	if strings.Contains(accept, "vnd.assetforge.v1") {
		return "v1"
	}
	if strings.Contains(accept, "vnd.assetforge.v2") {
		return "v2"
	}
	return ""
}
