package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseReplicaURLs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace", "   ", nil},
		{"single", "postgres://a", []string{"postgres://a"}},
		{"multiple", "postgres://a, postgres://b ,postgres://c", []string{"postgres://a", "postgres://b", "postgres://c"}},
		{"trailing comma", "postgres://a,", []string{"postgres://a"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseReplicaURLs(tc.in))
		})
	}
}

func TestLoadReplicaConfig(t *testing.T) {
	t.Setenv("DATABASE_REPLICA_URLS", "postgres://replica-1, postgres://replica-2")
	t.Setenv("DB_REPLICA_LAG_THRESHOLD", "45s")
	t.Setenv("DB_REPLICA_HEALTH_INTERVAL", "20s")
	t.Setenv("DB_REPLICA_FAILOVER_TO_PRIMARY", "false")
	t.Setenv("DB_REPLICA_MAX_IDLE_TIME", "2m")
	t.Setenv("DB_REPLICA_MAX_LIFETIME", "1h")

	cfg := LoadReplicaConfig()

	assert.True(t, cfg.Enabled())
	assert.Equal(t, []string{"postgres://replica-1", "postgres://replica-2"}, cfg.URLs)
	assert.Equal(t, 45*time.Second, cfg.LagThreshold)
	assert.Equal(t, 20*time.Second, cfg.HealthInterval)
	assert.False(t, cfg.FailoverToPrimary)
	assert.Equal(t, 2*time.Minute, cfg.MaxIdleReplicaAge)
	assert.Equal(t, time.Hour, cfg.MaxReplicaLifetime)
}

func TestLoadReplicaConfigDefaults(t *testing.T) {
	t.Setenv("DATABASE_REPLICA_URLS", "")
	t.Setenv("DB_REPLICA_LAG_THRESHOLD", "")
	t.Setenv("DB_REPLICA_HEALTH_INTERVAL", "")
	t.Setenv("DB_REPLICA_FAILOVER_TO_PRIMARY", "")

	cfg := LoadReplicaConfig()

	assert.False(t, cfg.Enabled())
	assert.Equal(t, 30*time.Second, cfg.LagThreshold)
	assert.Equal(t, 15*time.Second, cfg.HealthInterval)
	assert.True(t, cfg.FailoverToPrimary)
}
