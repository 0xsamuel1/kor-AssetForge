package config

import (
	"os"
	"strings"
	"time"
)

// ReplicaConfig configures PostgreSQL read replicas and query routing (#323).
//
// Read replicas let read-heavy endpoints scale independently of the primary
// (write) database. When no replicas are configured the router transparently
// falls back to the primary, so enabling this feature is backwards compatible.
type ReplicaConfig struct {
	// URLs is the list of read replica connection strings.
	URLs []string

	// LagThreshold marks a replica unhealthy when its replication lag exceeds
	// this value. Requests are then routed to the primary.
	LagThreshold time.Duration

	// HealthInterval is how often replica health and lag are probed.
	HealthInterval time.Duration

	// FailoverToPrimary routes reads to the primary when every replica is
	// unhealthy or lagging. When false, reads fail rather than silently
	// returning potentially stale data.
	FailoverToPrimary bool

	// MaxIdleReplicaAge closes idle replica connections after this duration.
	MaxIdleReplicaAge time.Duration

	// MaxReplicaLifetime is the maximum lifetime of a replica connection.
	MaxReplicaLifetime time.Duration
}

// LoadReplicaConfig reads replica configuration from the environment.
//
// Environment variables:
//
//	DATABASE_REPLICA_URLS          comma-separated replica DSNs
//	DB_REPLICA_LAG_THRESHOLD       max acceptable lag (default 30s)
//	DB_REPLICA_HEALTH_INTERVAL     lag probe interval (default 15s)
//	DB_REPLICA_FAILOVER_TO_PRIMARY route reads to primary on replica failure (default true)
//	DB_REPLICA_MAX_IDLE_TIME       replica connection max idle time (default 5m)
//	DB_REPLICA_MAX_LIFETIME        replica connection max lifetime (default 30m)
func LoadReplicaConfig() ReplicaConfig {
	return ReplicaConfig{
		URLs:               parseReplicaURLs(os.Getenv("DATABASE_REPLICA_URLS")),
		LagThreshold:       envDuration("DB_REPLICA_LAG_THRESHOLD", 30*time.Second),
		HealthInterval:     envDuration("DB_REPLICA_HEALTH_INTERVAL", 15*time.Second),
		FailoverToPrimary:  envBool("DB_REPLICA_FAILOVER_TO_PRIMARY", true),
		MaxIdleReplicaAge:  envDuration("DB_REPLICA_MAX_IDLE_TIME", 5*time.Minute),
		MaxReplicaLifetime: envDuration("DB_REPLICA_MAX_LIFETIME", 30*time.Minute),
	}
}

// Enabled reports whether at least one replica URL is configured.
func (c ReplicaConfig) Enabled() bool {
	return len(c.URLs) > 0
}

func parseReplicaURLs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	urls := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			urls = append(urls, trimmed)
		}
	}
	return urls
}
