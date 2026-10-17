package replica

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	replicaDomain = ".db.svc.wikimedia.cloud"
	// maxOpenPerPool bounds the connections to one wiki's cluster. The
	// replicas allow a handful of connections per tool account.
	maxOpenPerPool = 4
	// poolEvictAfter closes a wiki's pool after this long without use.
	poolEvictAfter = 10 * time.Minute
	// maxRetries bounds the retries of a query rejected because the
	// per-user connection limit was momentarily exceeded.
	maxRetries = 8
)

// Config tells how to reach the replicas.
type Config struct {
	User     string
	Password string
	// Host replaces every replica host name, e.g. 127.0.0.1 for local SSH
	// tunnels. Empty on Toolforge.
	Host string
	// Ports maps the leading labels of a replica host name, e.g. "enwiki" or
	// "links.commonswiki", to the local port of its tunnel. Only used with
	// Host.
	Ports map[string]int
	// Port is the port of every host not in Ports; 3306 if zero.
	Port int
}

type poolKey struct {
	wiki    string
	cluster Cluster
}

type pool struct {
	db       *sql.DB
	lastUsed time.Time
}

// Pools holds one lazily opened connection pool per wiki and cluster.
type Pools struct {
	cfg   Config
	mu    sync.Mutex
	pools map[poolKey]*pool
}

// NewPools returns an empty set of pools; connections are opened on first use.
func NewPools(cfg Config) *Pools {
	return &Pools{cfg: cfg, pools: map[poolKey]*pool{}}
}

// Close closes every pool.
func (p *Pools) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, pl := range p.pools {
		pl.db.Close()
		delete(p.pools, key)
	}
}

// address returns host:port and the database name for wiki on cluster.
func (p *Pools) address(wiki string, cluster Cluster) (string, string) {
	label := cluster.hostPrefix() + wiki
	host := label + "." + cluster.serverGroup() + replicaDomain
	port := p.cfg.Port
	if port == 0 {
		port = 3306
	}
	if p.cfg.Host != "" {
		host = p.cfg.Host
		if mapped, ok := p.cfg.Ports[label]; ok {
			port = mapped
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), wiki + "_p"
}

func (p *Pools) db(wiki string, cluster Cluster) (*sql.DB, error) {
	wiki = FixWikiName(wiki)
	cluster = cluster.effective(wiki)
	key := poolKey{wiki, cluster}
	now := time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()
	for k, pl := range p.pools {
		if now.Sub(pl.lastUsed) > poolEvictAfter {
			pl.db.Close()
			delete(p.pools, k)
		}
	}
	if pl, ok := p.pools[key]; ok {
		pl.lastUsed = now
		return pl.db, nil
	}

	addr, dbName := p.address(wiki, cluster)
	cfg := mysql.NewConfig()
	cfg.User = p.cfg.User
	cfg.Passwd = p.cfg.Password
	cfg.Net = "tcp"
	cfg.Addr = addr
	cfg.DBName = dbName
	cfg.Timeout = 10 * time.Second
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("database config for %s: %w", wiki, err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(maxOpenPerPool)
	db.SetMaxIdleConns(2)
	// The replicas ask tools not to hold idle connections.
	db.SetConnMaxIdleTime(30 * time.Second)
	db.SetConnMaxLifetime(5 * time.Minute)
	p.pools[key] = &pool{db: db, lastUsed: now}
	return db, nil
}

// Query runs q against wiki's cluster and calls scan once per result row.
func (p *Pools) Query(ctx context.Context, wiki string, cluster Cluster, q *Query, scan func(*sql.Rows) error) error {
	db, err := p.db(wiki, cluster)
	if err != nil {
		return err
	}
	start := time.Now()
	for attempt := 0; ; attempt++ {
		err = runQuery(ctx, db, q, scan)
		if err == nil || !isConnectionLimit(err) || attempt >= maxRetries {
			break
		}
		delay := time.Duration(100<<attempt) * time.Millisecond
		delay = min(delay, 5*time.Second)
		delay = time.Duration(float64(delay) * (0.75 + rand.Float64()/2))
		slog.Warn("replica connection limit reached, retrying", "wiki", wiki, "attempt", attempt+1, "delay", delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	slog.Debug("replica query", "wiki", wiki, "cluster", cluster, "args", len(q.Args()), "duration", time.Since(start), "error", err)
	if err != nil {
		return fmt.Errorf("database query on %s: %w", wiki, err)
	}
	return nil
}

func runQuery(ctx context.Context, db *sql.DB, q *Query, scan func(*sql.Rows) error) error {
	// With arguments and the driver's default interpolateParams=false, this
	// is a server-side prepared statement.
	rows, err := db.QueryContext(ctx, q.SQL(), q.Args()...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// isConnectionLimit reports whether err is MariaDB refusing a connection over
// the per-user limit (ER_TOO_MANY_USER_CONNECTIONS, ER_USER_LIMIT_REACHED).
func isConnectionLimit(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1203 || me.Number == 1226)
}
