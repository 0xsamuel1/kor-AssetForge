package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/yourusername/kor-assetforge/config"
)

func openSQLite(t *testing.T, name string) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)").Error)
	return db
}

func insertWidget(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO widgets (name) VALUES (?)", name).Error)
}

func widgetNames(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var names []string
	require.NoError(t, db.Raw("SELECT name FROM widgets ORDER BY name").Scan(&names).Error)
	return names
}

func TestIsReadQuery(t *testing.T) {
	readQueries := []string{
		"SELECT * FROM users",
		"  select id from assets where id = 1",
		"-- leading comment\nSELECT 1",
		"/* block */ SELECT now()",
		"WITH recent AS (SELECT * FROM assets) SELECT * FROM recent",
		"SHOW TABLES",
		"EXPLAIN SELECT 1",
		"(SELECT 1)",
	}
	for _, q := range readQueries {
		assert.Truef(t, IsReadQuery(q), "expected read query: %q", q)
	}

	writeQueries := []string{
		"INSERT INTO users (id) VALUES (1)",
		"UPDATE users SET name = 'x'",
		"DELETE FROM users",
		"CREATE TABLE foo (id int)",
		"ALTER TABLE users ADD COLUMN age int",
		"DROP TABLE users",
		"TRUNCATE users",
		"WITH x AS (SELECT 1) INSERT INTO t SELECT * FROM x",
		"",
	}
	for _, q := range writeQueries {
		assert.Falsef(t, IsReadQuery(q), "expected write query: %q", q)
	}
}

func TestRouterExplicitReadWriteSplit(t *testing.T) {
	primary := openSQLite(t, "primary.db")
	replica := openSQLite(t, "replica.db")

	insertWidget(t, primary, "primary-only")
	insertWidget(t, replica, "replica-only")

	cluster := NewCluster(primary, replica)
	router := NewRouter(cluster)

	// Read path -> replica sees only the replica row.
	assert.Equal(t, []string{"replica-only"}, widgetNames(t, router.Read()))

	// Write path -> primary sees only the primary row.
	assert.Equal(t, []string{"primary-only"}, widgetNames(t, router.Write()))

	// SQL classification selects the correct path automatically.
	assert.Equal(t, []string{"replica-only"}, widgetNames(t, router.DB("SELECT * FROM widgets")))
	assert.Equal(t, []string{"primary-only"}, widgetNames(t, router.DB("UPDATE widgets SET name = name")))
}

func TestRouterAutomaticReadWriteSplit(t *testing.T) {
	primary := openSQLite(t, "primary.db")
	replica := openSQLite(t, "replica.db")

	insertWidget(t, replica, "replica-only")

	cluster := NewCluster(primary, replica)
	router := NewRouter(cluster)

	db, err := router.Auto()
	require.NoError(t, err)

	// A write must stay on the primary.
	require.NoError(t, db.Exec("INSERT INTO widgets (name) VALUES (?)", "primary-insert").Error)
	// The primary handle has the plugin installed, so pin the read to inspect it.
	assert.Equal(t, []string{"primary-insert"}, widgetNames(t, UsePrimary(primary)))
	assert.Equal(t, []string{"replica-only"}, widgetNames(t, replica))

	// A model/raw read is routed to the replica automatically.
	assert.Equal(t, []string{"replica-only"}, widgetNames(t, db))

	// UsePrimary pins a read to the primary for read-your-writes consistency.
	pinned := UsePrimary(db.Session(&gorm.Session{})).Raw("SELECT name FROM widgets")
	assert.Equal(t, []string{"primary-insert"}, widgetNames(t, pinned))
}

func TestRouterFailsOverToPrimary(t *testing.T) {
	primary := openSQLite(t, "primary.db")
	replica := openSQLite(t, "replica.db")
	insertWidget(t, primary, "primary-only")
	insertWidget(t, replica, "replica-only")

	cluster := NewCluster(primary, replica)
	router := NewRouter(cluster)

	require.Same(t, replica, router.Read())

	// Explicitly unhealthy replica -> fall back to primary.
	require.NotNil(t, cluster.ReplicaNode())
	require.True(t, cluster.SetReplicaHealth("replica-1", false))
	require.False(t, cluster.ReplicaNode() != nil)
	require.Same(t, primary, router.Read())
	require.Equal(t, []string{"primary-only"}, widgetNames(t, router.Read()))

	// Recover the replica.
	require.True(t, cluster.SetReplicaHealth("replica-1", true))
	require.Same(t, replica, router.Read())
	assert.Equal(t, 1, cluster.HealthyReplicas())
}

func TestRecordLagMarksReplicaUnhealthy(t *testing.T) {
	primary := openSQLite(t, "primary.db")
	replica := openSQLite(t, "replica.db")

	cfgLag := 10 * time.Second
	cluster := NewClusterWithConfig(primary, config.ReplicaConfig{
		LagThreshold:      cfgLag,
		FailoverToPrimary: true,
	}, replica)
	router := NewRouter(cluster)

	require.Same(t, replica, router.Read())

	// Lag above threshold -> replica marked unhealthy, reads use primary.
	cluster.RecordLag("replica-1", 30*time.Second)
	require.Same(t, primary, router.Read())
	require.Equal(t, 0, cluster.HealthyReplicas())
	assert.Equal(t, 30*time.Second, cluster.Replicas()[0].Lag())

	// Lag back within threshold -> replica becomes usable again.
	cluster.RecordLag("replica-1", 2*time.Second)
	require.Same(t, replica, router.Read())
	assert.Equal(t, 1, cluster.HealthyReplicas())
}

func TestClusterWithNoReplicasUsesPrimary(t *testing.T) {
	primary := openSQLite(t, "primary.db")
	insertWidget(t, primary, "primary-only")

	cluster := NewCluster(primary)
	require.False(t, cluster.HasReplicas())
	require.Same(t, primary, cluster.Replica())
	require.Same(t, primary, NewRouter(cluster).Read())
}
