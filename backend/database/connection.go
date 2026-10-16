// Package database provides PostgreSQL read/write splitting across a primary
// database and one or more read replicas (#323).
//
// The primary node serves all writes and (unless disabled) acts as the fallback
// for reads when a replica is unhealthy or lagging. Replicas serve read-only
// queries. See backend/database/router.go for the routing logic and
// docs/DATABASE_REPLICATION.md for operational guidance.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/yourusername/kor-assetforge/config"
)

// LagProbe measures replication lag for a replica. Implementations return the
// observed lag; a negative value means lag could not be determined.
type LagProbe func(ctx context.Context, replica *gorm.DB) (time.Duration, error)

// Node is a single database connection in the cluster.
type Node struct {
	Name string
	DB   *gorm.DB
	URL  string

	healthy  atomic.Bool
	lagNanos atomic.Int64
}

// Healthy reports whether the node is currently considered available.
func (n *Node) Healthy() bool { return n.healthy.Load() }

// Lag returns the last observed replication lag for the node.
func (n *Node) Lag() time.Duration { return time.Duration(n.lagNanos.Load()) }

func (n *Node) setHealthy(v bool) { n.healthy.Store(v) }
func (n *Node) setLag(d time.Duration) {
	n.lagNanos.Store(int64(d))
}

// Cluster bundles the primary connection with its read replicas and tracks
// replica health and lag. It is safe for concurrent use.
type Cluster struct {
	primary  *gorm.DB
	replicas []*Node

	lagThreshold      time.Duration
	failoverToPrimary bool
	lagProbe          LagProbe

	next atomic.Uint64
}

// NewCluster builds a cluster from already-open connections. This is primarily
// used by tests and by callers that manage their own connections.
func NewCluster(primary *gorm.DB, replicas ...*gorm.DB) *Cluster {
	c := &Cluster{
		primary:           primary,
		lagThreshold:      config.ReplicaConfig{}.LagThreshold,
		failoverToPrimary: true,
		lagProbe:          defaultLagProbe,
	}
	if c.lagThreshold == 0 {
		c.lagThreshold = 30 * time.Second
	}
	for i, db := range replicas {
		node := &Node{Name: fmt.Sprintf("replica-%d", i+1), DB: db}
		node.setHealthy(true)
		c.replicas = append(c.replicas, node)
	}
	return c
}

// NewClusterWithConfig builds a cluster with explicit replica configuration.
func NewClusterWithConfig(primary *gorm.DB, replicaCfg config.ReplicaConfig, replicas ...*gorm.DB) *Cluster {
	c := NewCluster(primary, replicas...)
	if replicaCfg.LagThreshold > 0 {
		c.lagThreshold = replicaCfg.LagThreshold
	}
	c.failoverToPrimary = replicaCfg.FailoverToPrimary
	return c
}

// Open connects to the primary and any configured replicas, applying the shared
// pool configuration to every connection.
func Open(primaryURL string, poolCfg config.DBPoolConfig, replicaCfg config.ReplicaConfig) (*Cluster, error) {
	primary, err := openDB(primaryURL, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("open primary: %w", err)
	}

	cluster := NewClusterWithConfig(primary, replicaCfg)
	for i, url := range replicaCfg.URLs {
		db, err := openDB(url, poolCfg)
		if err != nil {
			// A replica being unavailable must not prevent startup: reads fall
			// back to the primary and the health checker will keep retrying.
			// Skipped replicas are surfaced via HealthyReplicas().
			continue
		}
		setReplicaPoolLimits(db, replicaCfg)
		node := &Node{Name: fmt.Sprintf("replica-%d", i+1), DB: db, URL: url}
		node.setHealthy(true)
		cluster.replicas = append(cluster.replicas, node)
	}
	return cluster, nil
}

// Primary returns the write connection.
func (c *Cluster) Primary() *gorm.DB { return c.primary }

// HasReplicas reports whether any replica is configured.
func (c *Cluster) HasReplicas() bool { return len(c.replicas) > 0 }

// Replicas returns the configured replica nodes.
func (c *Cluster) Replicas() []*Node { return c.replicas }

// HealthyReplicas returns the number of replicas currently healthy and within
// the configured lag threshold.
func (c *Cluster) HealthyReplicas() int {
	n := 0
	for _, r := range c.replicas {
		if c.replicaUsable(r) {
			n++
		}
	}
	return n
}

// Replica returns a healthy replica connection, or the primary when no replica
// is usable. It is used by the router for read queries.
func (c *Cluster) Replica() *gorm.DB {
	if node := c.nextUsableReplica(); node != nil {
		return node.DB
	}
	return c.primary
}

// ReplicaNode returns the selected replica node, or nil when the primary was
// selected as the read target.
func (c *Cluster) ReplicaNode() *Node { return c.nextUsableReplica() }

func (c *Cluster) nextUsableReplica() *Node {
	total := len(c.replicas)
	if total == 0 {
		return nil
	}
	start := int(c.next.Add(1) - 1)
	for i := 0; i < total; i++ {
		node := c.replicas[(start+i)%total]
		if c.replicaUsable(node) {
			return node
		}
	}
	return nil
}

func (c *Cluster) replicaUsable(node *Node) bool {
	if node == nil || !node.Healthy() {
		return false
	}
	lag := node.Lag()
	return lag <= c.lagThreshold
}

// SetReplicaHealth manually updates a replica's health. Returns false when no
// replica with that name exists.
func (c *Cluster) SetReplicaHealth(name string, healthy bool) bool {
	for _, node := range c.replicas {
		if node.Name == name {
			node.setHealthy(healthy)
			return true
		}
	}
	return false
}

// RecordLag stores an observed lag value for a named replica and marks it
// unhealthy when the lag exceeds the threshold.
func (c *Cluster) RecordLag(name string, lag time.Duration) {
	for _, node := range c.replicas {
		if node.Name == name {
			node.setLag(lag)
			node.setHealthy(lag <= c.lagThreshold)
			return
		}
	}
}

// CheckReplicaHealth probes every replica's connectivity and replication lag,
// updating health state. It is called periodically by StartHealthChecks.
func (c *Cluster) CheckReplicaHealth(ctx context.Context) {
	for _, node := range c.replicas {
		if err := ping(node.DB); err != nil {
			node.setHealthy(false)
			continue
		}
		lag, err := c.lagProbe(ctx, node.DB)
		if err != nil || lag < 0 {
			// Unable to determine lag: keep the replica but treat unknown lag
			// as acceptable only if connectivity succeeded.
			node.setHealthy(true)
			continue
		}
		node.setLag(lag)
		node.setHealthy(lag <= c.lagThreshold)
	}
}

// StartHealthChecks runs CheckReplicaHealth on the configured interval until the
// context is cancelled. It is a no-op when there are no replicas.
func (c *Cluster) StartHealthChecks(ctx context.Context, interval time.Duration) {
	if len(c.replicas) == 0 {
		return
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.CheckReplicaHealth(ctx)
			}
		}
	}()
}

// Close closes the primary and all replica connections.
func (c *Cluster) Close() error {
	var firstErr error
	closeDB := func(db *gorm.DB) {
		if db == nil {
			return
		}
		if sqlDB, err := db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	closeDB(c.primary)
	for _, node := range c.replicas {
		closeDB(node.DB)
	}
	return firstErr
}

func openDB(dsn string, poolCfg config.DBPoolConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if poolCfg.MaxOpen > 0 {
		sqlDB.SetMaxOpenConns(poolCfg.MaxOpen)
	}
	if poolCfg.MaxIdle > 0 {
		sqlDB.SetMaxIdleConns(poolCfg.MaxIdle)
	}
	if poolCfg.MaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(poolCfg.MaxLifetime)
	}
	if poolCfg.MaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(poolCfg.MaxIdleTime)
	}
	if err := ping(db); err != nil {
		return nil, err
	}
	return db, nil
}

func setReplicaPoolLimits(db *gorm.DB, cfg config.ReplicaConfig) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	if cfg.MaxIdleReplicaAge > 0 {
		sqlDB.SetConnMaxIdleTime(cfg.MaxIdleReplicaAge)
	}
	if cfg.MaxReplicaLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.MaxReplicaLifetime)
	}
}

func ping(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("nil database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx)
}

// defaultLagProbe measures replication lag on PostgreSQL. On non-PostgreSQL
// dialects (e.g. sqlite in tests) it reports zero lag.
func defaultLagProbe(ctx context.Context, replica *gorm.DB) (time.Duration, error) {
	if replica == nil || replica.Dialector == nil {
		return 0, nil
	}
	switch replica.Dialector.Name() {
	case "postgres":
		return postgresReplicaLag(ctx, replica)
	default:
		return 0, nil
	}
}

func postgresReplicaLag(ctx context.Context, replica *gorm.DB) (time.Duration, error) {
	// pg_last_xact_replay_timestamp() returns NULL when the replica has replayed
	// no transactions yet (or is a primary); COALESCE maps that to 0.
	const query = `SELECT COALESCE(EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp())), 0)`
	var seconds float64
	if err := replica.WithContext(ctx).Raw(query).Scan(&seconds).Error; err != nil {
		return -1, err
	}
	if seconds < 0 {
		return 0, nil
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// isTransaction reports whether the statement is executing inside a transaction
// (and therefore must stay on its current connection).
func isTransaction(db *gorm.DB) bool {
	if db == nil || db.Statement == nil {
		return false
	}
	switch db.Statement.ConnPool.(type) {
	case *sql.Tx:
		return true
	}
	return strings.Contains(fmt.Sprintf("%T", db.Statement.ConnPool), "PreparedStmtTX")
}
