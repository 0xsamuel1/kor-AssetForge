package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

// ipWhitelistCache holds the in-memory copy of allowed networks and when it was
// last refreshed from the database.
type ipWhitelistCache struct {
	mu          sync.RWMutex
	nets        []*net.IPNet
	lastRefresh time.Time
	ttl         time.Duration
}

var globalIPWhitelistCache = &ipWhitelistCache{ttl: 60 * time.Second}

func (c *ipWhitelistCache) load(db *gorm.DB) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.lastRefresh) < c.ttl {
		return
	}

	var entries []models.IPWhitelistEntry
	if err := db.Find(&entries).Error; err != nil {
		return
	}

	var nets []*net.IPNet
	for _, e := range entries {
		_, ipNet, err := net.ParseCIDR(e.CIDR)
		if err == nil {
			nets = append(nets, ipNet)
		}
	}
	c.nets = nets
	c.lastRefresh = time.Now()
}

func (c *ipWhitelistCache) contains(ip net.IP) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, n := range c.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IPWhitelist returns a middleware that restricts access to IPs present in the
// database whitelist. The list is cached for 60 seconds to avoid hammering the
// database on every request.
func IPWhitelist(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		globalIPWhitelistCache.load(db)

		clientIP := net.ParseIP(c.ClientIP())
		if clientIP == nil || !globalIPWhitelistCache.contains(clientIP) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Access denied: your IP address is not whitelisted",
			})
			return
		}
		c.Next()
	}
}
