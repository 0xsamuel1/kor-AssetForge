package database

import (
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

// forcePrimaryKey is the GORM statement setting used by UsePrimary to pin a
// session/query to the write connection.
const forcePrimaryKey = "assetforge:rw_force_primary"

// writeKeyword matches SQL keywords that indicate a statement mutates data or
// schema. Read-only statements are never matched.
var writeKeyword = regexp.MustCompile(
	`(?i)\b(INSERT|UPDATE|DELETE|MERGE|UPSERT|TRUNCATE|ALTER|CREATE|DROP|GRANT|REVOKE|REPLACE|VACUUM|REINDEX|CLUSTER|REFRESH|COMMENT|COPY|CALL|DO|SET|RESET|LOCK)\b`,
)

// Router decides whether a query should be served by a read replica or the
// primary database.
type Router struct {
	cluster *Cluster
}

// NewRouter creates a Router for the given cluster.
func NewRouter(cluster *Cluster) *Router {
	return &Router{cluster: cluster}
}

// Cluster returns the underlying cluster.
func (r *Router) Cluster() *Cluster { return r.cluster }

// Read returns a connection suitable for read-only queries. It prefers a
// healthy, non-lagging replica and falls back to the primary.
func (r *Router) Read() *gorm.DB {
	if r.cluster == nil {
		return nil
	}
	return r.cluster.Replica()
}

// Write returns the primary write connection.
func (r *Router) Write() *gorm.DB {
	if r.cluster == nil {
		return nil
	}
	return r.cluster.Primary()
}

// DB classifies the supplied SQL and returns the appropriate connection: a
// replica for read-only statements, the primary otherwise.
func (r *Router) DB(sql string) *gorm.DB {
	if IsReadQuery(sql) {
		return r.Read()
	}
	return r.Write()
}

// Auto returns the primary connection with the read/write splitting plugin
// installed, so that normal GORM operations (Find, First, Create, ...) are
// routed automatically. UsePrimary can pin a statement to the primary.
func (r *Router) Auto() (*gorm.DB, error) {
	if r.cluster == nil || r.cluster.Primary() == nil {
		return nil, fmt.Errorf("database: nil cluster or primary")
	}
	primary := r.cluster.Primary()
	if err := primary.Use(NewReadWritePlugin(r.cluster)); err != nil {
		return nil, err
	}
	return primary, nil
}

// UsePrimary pins the current GORM session to the primary connection so the
// following operation is not routed to a replica. Use it for read-your-writes
// consistency after a mutation.
func UsePrimary(db *gorm.DB) *gorm.DB {
	return db.Set(forcePrimaryKey, true)
}

// ReadWritePlugin is a GORM plugin that routes read operations to replicas.
type ReadWritePlugin struct {
	cluster *Cluster
}

// NewReadWritePlugin creates the automatic read/write splitting plugin.
func NewReadWritePlugin(cluster *Cluster) *ReadWritePlugin {
	return &ReadWritePlugin{cluster: cluster}
}

// Name implements gorm.Plugin.
func (p *ReadWritePlugin) Name() string { return "assetforge:read-write-split" }

// Initialize implements gorm.Plugin by registering routing callbacks.
func (p *ReadWritePlugin) Initialize(db *gorm.DB) error {
	if p.cluster == nil || !p.cluster.HasReplicas() {
		return nil
	}
	if err := db.Callback().Query().Before("gorm:query").
		Register("assetforge:route_query", p.routeRead); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").
		Register("assetforge:route_row", p.routeRead); err != nil {
		return err
	}
	return db.Callback().Raw().Before("gorm:raw").
		Register("assetforge:route_raw", p.routeRaw)
}

func (p *ReadWritePlugin) routeRead(tx *gorm.DB) {
	if !p.shouldRoute(tx) {
		return
	}
	p.routeToReplica(tx)
}

func (p *ReadWritePlugin) routeRaw(tx *gorm.DB) {
	if !p.shouldRoute(tx) {
		return
	}
	if tx.Statement == nil || !IsReadQuery(tx.Statement.SQL.String()) {
		return
	}
	p.routeToReplica(tx)
}

func (p *ReadWritePlugin) routeToReplica(tx *gorm.DB) {
	primary := p.cluster.Primary()
	replica := p.cluster.Replica()
	if replica == nil || primary == nil || replica == primary {
		return
	}
	tx.Statement.ConnPool = replica.ConnPool
}

func (p *ReadWritePlugin) shouldRoute(tx *gorm.DB) bool {
	if tx == nil || tx.Statement == nil {
		return false
	}
	if pinned, ok := tx.Get(forcePrimaryKey); ok {
		if force, _ := pinned.(bool); force {
			return false
		}
	}
	// Never reroute statements already bound to a transaction.
	return !isTransaction(tx)
}

// IsReadQuery reports whether sql is a read-only statement. It is conservative:
// anything that is not clearly a read is treated as a write so it goes to the
// primary.
func IsReadQuery(sql string) bool {
	normalized := normalizeSQL(sql)
	if normalized == "" {
		return false
	}

	keyword := firstKeyword(normalized)
	switch keyword {
	case "SELECT", "SHOW", "EXPLAIN", "VALUES", "TABLE":
		return true
	case "WITH":
		// A CTE is read-only only when it contains no write keyword.
		return !writeKeyword.MatchString(normalized)
	default:
		return false
	}
}

// normalizeSQL trims leading whitespace and comments from a statement.
func normalizeSQL(sql string) string {
	s := strings.TrimSpace(sql)
	for {
		switch {
		case strings.HasPrefix(s, "--"):
			if idx := strings.IndexByte(s, '\n'); idx >= 0 {
				s = strings.TrimSpace(s[idx+1:])
				continue
			}
			return ""
		case strings.HasPrefix(s, "/*"):
			idx := strings.Index(s, "*/")
			if idx < 0 {
				return ""
			}
			s = strings.TrimSpace(s[idx+2:])
			continue
		}
		break
	}
	// Tolerate leading parentheses around a statement, e.g. "(SELECT ...)".
	s = strings.TrimLeft(s, "(")
	return strings.TrimSpace(s)
}

// firstKeyword returns the first whitespace-delimited token, uppercased.
func firstKeyword(sql string) string {
	fields := strings.Fields(sql)
	if len(fields) == 0 {
		return ""
	}
	token := strings.TrimRight(fields[0], "(;")
	return strings.ToUpper(token)
}
