package petscan

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

const (
	// pageBatchSize is the number of titles or IDs in one IN list.
	pageBatchSize = 15000
	// maxParallelQueries bounds the concurrent database queries of one run.
	// The replicas allow only a few connections per tool account.
	maxParallelQueries = 4
)

// Env holds the services queries run against. It is shared by all runs.
type Env struct {
	DB   *replica.Pools
	Wiki *wiki.Client
	// MaxResults is the most pages one query may load.
	MaxResults int
	// UseFileTable reads file metadata from the new file/filerevision
	// tables instead of the image_compat view.
	UseFileTable bool
	// PublicURL is the base URL of the web frontend, e.g.
	// https://dinoscan.toolforge.org/, used for links in the output.
	PublicURL string
}

// Run is one PetScan query being executed.
type Run struct {
	env         *Env
	params      Params
	matrix      *wiki.SiteMatrix
	combination *Combination
	// wikiBySource is the wiki of each data source's result.
	wikiBySource map[string]string
	result       *PageList
	queryTime    time.Duration
}

// NewRun prepares a query.
func NewRun(env *Env, params Params) *Run {
	return &Run{env: env, params: params, wikiBySource: map[string]string{}}
}

// source is a data source providing an initial list of pages.
type source struct {
	name   string
	canRun func(p Params) bool
	run    func(ctx context.Context, r *Run) (*PageList, error)
}

// sources are the primary data sources, in the order their results are
// combined by default.
var sources = []source{
	{"categories", canRunDatabase, runDatabase},
	{"sparql", canRunSPARQL, runSPARQL},
	{"manual", canRunManual, runManual},
	{"pagepile", canRunPagePile, runPagePile},
	{"search", canRunSearch, runSearch},
	{"wikidata", canRunWikidata, runWikidata},
}

// sitelinksSource runs only when no other source applies.
var sitelinksSource = source{"sitelinks", canRunSitelinks, runSitelinks}

// Execute runs the query; Result returns its outcome.
func (r *Run) Execute(ctx context.Context) error {
	start := time.Now()
	if r.params.Has("wdf_main") {
		return invalidf("WD-FIST is not supported, please use PetScan")
	}
	matrix, err := r.env.Wiki.SiteMatrix(ctx)
	if err != nil {
		return upstream(err)
	}
	r.matrix = matrix

	var applicable []source
	for _, s := range sources {
		if s.canRun(r.params) {
			applicable = append(applicable, s)
		}
	}
	if len(applicable) == 0 && sitelinksSource.canRun(r.params) {
		applicable = append(applicable, sitelinksSource)
	}
	if len(applicable) == 0 {
		return invalidf("no possible data source found in parameters")
	}

	results := make([]*PageList, len(applicable))
	err = parallel(ctx, len(applicable), len(applicable), func(ctx context.Context, i int) error {
		pl, err := applicable[i].run(ctx, r)
		if err != nil {
			return err
		}
		results[i] = pl
		return pl.checkLimit(r.env.MaxResults)
	})
	if err != nil {
		return err
	}

	byName := map[string]*PageList{}
	var names []string
	for i, s := range applicable {
		byName[s.name] = results[i]
		names = append(names, s.name)
		r.wikiBySource[s.name] = results[i].Wiki
		slog.Debug("source done", "source", s.name, "wiki", results[i].Wiki, "pages", results[i].Len())
	}

	if r.params.Has("source_combination") {
		r.combination = parseCombination(r.params.Get("source_combination"))
	} else {
		r.combination = defaultCombination(names)
	}
	result, err := r.combine(ctx, r.combination, byName)
	if err != nil {
		return err
	}
	if err := result.checkLimit(r.env.MaxResults); err != nil {
		return err
	}
	r.result = result

	if err := r.postProcess(ctx, names); err != nil {
		return err
	}
	r.queryTime = time.Since(start)
	return nil
}

// mainWiki returns the wiki given by the language and project parameters.
func (r *Run) mainWiki() (string, error) {
	w, ok := r.params.MainWiki(r.matrix)
	if !ok {
		return "", invalidf("unknown wiki for language %q and project %q", r.params.Get("language"), r.params.Get("project"))
	}
	return w, nil
}

// query runs one database query; see replica.Pools.Query.
func (r *Run) query(ctx context.Context, w string, cluster replica.Cluster, q *replica.Query, scan func(*sql.Rows) error) error {
	return upstream(r.env.DB.Query(ctx, w, cluster, q, scan))
}

// queryAll runs queries on one wiki's cluster, a few at a time. Calls of scan
// are serialized, so it may update shared state without locking.
func (r *Run) queryAll(ctx context.Context, w string, cluster replica.Cluster, queries []*replica.Query, scan func(*sql.Rows) error) error {
	var mu sync.Mutex
	locked := func(rows *sql.Rows) error {
		mu.Lock()
		defer mu.Unlock()
		return scan(rows)
	}
	return parallel(ctx, len(queries), maxParallelQueries, func(ctx context.Context, i int) error {
		return r.query(ctx, w, cluster, queries[i], locked)
	})
}

// parallel calls fn for 0…n-1 with at most limit calls at a time. It returns
// the first error and cancels the context of the other calls.
func parallel(ctx context.Context, n, limit int, fn func(ctx context.Context, i int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, max(limit, 1))
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for i := range n {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(ctx, i); err != nil {
				once.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

// siteInfo returns the namespaces etc. of w.
func (r *Run) siteInfo(ctx context.Context, w string) (*wiki.SiteInfo, error) {
	info, err := r.env.Wiki.SiteInfo(ctx, w)
	if err != nil && !errors.Is(err, wiki.ErrUnknownWiki) {
		err = upstream(err)
	}
	return info, err
}
