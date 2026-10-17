package petscan

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// The "categories" source and the database filters.
//
// One base query selects pages from a primary source — category members, or
// all of page narrowed down by filter clauses — and filter clauses add
// conditions on templates, links, page properties, revisions etc.
//
// Commons keeps its links tables on another host than revision, page_props
// and friends, so one statement cannot filter on both. Clauses the base
// query's cluster cannot serve are deferred: applied afterwards as
// "page_id IN (…)" passes on the cluster that can. Off Commons there is only
// one cluster and nothing is deferred.

const (
	// maxSubcategories bounds a category tree.
	maxSubcategories = 500000
	// categoryBatchSize is the number of category titles in one IN list.
	categoryBatchSize = 25000
	// maxPlaceholders stays below MariaDB's limit of 65535 per statement.
	maxPlaceholders = 60000
	// maxSeedPages is the most pages a seed may name and still restrict the
	// base query; see filter.seed.
	maxSeedPages = 150000
	// trackingCategoriesItem is Wikidata's "Category:Tracking categories";
	// its sitelinks name each wiki's container of tracking categories.
	trackingCategoriesItem = 6964088
)

// Tables each family of filter clauses reads besides page.
var (
	tablesTemplateLinks = []string{"templatelinks", "linktarget"}
	tablesPageLinks     = []string{"pagelinks", "linktarget"}
	tablesPageProps     = []string{"page_props"}
	tablesRevision      = []string{"revision"}
	tablesRevisionActor = []string{"revision", "actor"}
	tablesRevisionBots  = []string{"revision", "actor", "user_groups"}
	tablesFlaggedPages  = []string{"flaggedpages"}
	tablesORES          = []string{"ores_classification", "ores_model"}
	tablesCategoryLinks = []string{"page", "categorylinks", "linktarget"}
)

func canRunDatabase(p Params) bool {
	for _, key := range []string{"categories", "templates_yes", "templates_any", "outlinks_yes", "outlinks_any", "links_to_all", "links_to_any", "created_by"} {
		if p.Has(key) {
			return true
		}
	}
	return false
}

func runDatabase(ctx context.Context, r *Run) (*PageList, error) {
	db, err := newDatabaseQuery(ctx, r)
	if err != nil {
		return nil, err
	}
	return db.pages(ctx, nil)
}

// databaseQuery is the categories source, or the database filters applied
// to the result of other sources.
type databaseQuery struct {
	run  *Run
	wiki string
	site *wiki.SiteInfo

	combineUnion bool // categories: union instead of intersection
	depth        int
	catPos       []string
	catNeg       []string
	namespaces   []int

	templatesYes, templatesAny, templatesNo []string
	templatesYesTalk                        bool
	templatesAnyTalk                        bool
	templatesNoTalk                         bool

	linkedFromAll, linkedFromAny, linkedFromNone []string
	linksToAll, linksToAny, linksToNone          []string

	pageImage             string
	oresType              string
	oresPrediction        string
	oresProbFrom          *float64
	oresProbTo            *float64
	lastEditBot           string
	lastEditAnon          string
	lastEditFlagged       string
	redirects             string
	softRedirects         string
	disambiguation        string
	talkPageExists        string
	wikidataItem          string
	larger, smaller       *int
	sinceRev0             *int
	minLinks, maxLinks    *int
	maxAgeHours           *int
	onlyNew               bool
	before, after         string
	createdBy             []string
	skipTrackingCats      bool
	skipHiddenCats        bool
	trackingCategory      string // resolved local name, if skipTrackingCats
	categoryTreesPos      [][]string
	categoryTreesNeg      [][]string
	categoryCaseSensitive bool
}

func newDatabaseQuery(ctx context.Context, r *Run) (*databaseQuery, error) {
	w, err := r.mainWiki()
	if err != nil {
		return nil, err
	}
	return newDatabaseQueryFor(ctx, r, w)
}

func newDatabaseQueryFor(ctx context.Context, r *Run, w string) (*databaseQuery, error) {
	site, err := r.siteInfo(ctx, w)
	if err != nil {
		return nil, err
	}
	p := r.params
	d := &databaseQuery{
		run:              r,
		wiki:             w,
		site:             site,
		depth:            parseDepth(p.GetDefault("depth", "0")),
		catPos:           p.List("categories", "\n"),
		catNeg:           p.List("negcats", "\n"),
		namespaces:       p.Namespaces,
		templatesYesTalk: p.Has("templates_use_talk_yes"),
		templatesAnyTalk: p.Has("templates_use_talk_any"),
		templatesNoTalk:  p.Has("templates_use_talk_no"),
		linkedFromAll:    p.List("outlinks_yes", "\n"),
		linkedFromAny:    p.List("outlinks_any", "\n"),
		linkedFromNone:   p.List("outlinks_no", "\n"),
		linksToAll:       p.List("links_to_all", "\n"),
		linksToAny:       p.List("links_to_any", "\n"),
		linksToNone:      p.List("links_to_no", "\n"),
		pageImage:        p.GetDefault("page_image", "any"),
		oresType:         p.Get("ores_type"),
		oresPrediction:   p.GetDefault("ores_prediction", "any"),
		lastEditBot:      p.GetDefault("edits[bots]", "both"),
		lastEditAnon:     p.GetDefault("edits[anons]", "both"),
		lastEditFlagged:  p.GetDefault("edits[flagged]", "both"),
		redirects:        p.Get("show_redirects"),
		softRedirects:    p.Get("show_soft_redirects"),
		disambiguation:   p.Get("show_disambiguation_pages"),
		talkPageExists:   p.GetDefault("talk_page_exists", "both"),
		wikidataItem:     p.GetDefault("wikidata_item", "any"),
		onlyNew:          p.Has("only_new"),
		before:           p.Get("before"),
		after:            p.Get("after"),
		createdBy:        p.List("created_by", "\n"),
		skipTrackingCats: p.Has("skip_tracking_categories"),
		skipHiddenCats:   p.Has("skip_hidden_categories"),
	}
	// A single category is cheaper as a union, and the same.
	d.combineUnion = p.Get("combination") == "union" || len(d.catPos) == 1
	d.categoryCaseSensitive = site.CaseSensitive(14)
	templateCaseSensitive := site.CaseSensitive(10)
	d.templatesYes = normalizeTitles(p.List("templates_yes", "\n"), templateCaseSensitive)
	d.templatesAny = normalizeTitles(p.List("templates_any", "\n"), templateCaseSensitive)
	d.templatesNo = normalizeTitles(p.List("templates_no", "\n"), templateCaseSensitive)

	d.larger = optInt(p, "larger")
	d.smaller = optInt(p, "smaller")
	d.sinceRev0 = optInt(p, "since_rev0")
	d.minLinks = optInt(p, "minlinks")
	d.maxLinks = optInt(p, "maxlinks")
	if p.Has("max_age") {
		hours, _ := strconv.Atoi(p.Get("max_age"))
		d.maxAgeHours = &hours
	}
	if p.Has("ores_prob_from") {
		v, err := strconv.ParseFloat(p.Get("ores_prob_from"), 64)
		if err != nil {
			v = 0
		}
		d.oresProbFrom = &v
	}
	if p.Has("ores_prob_to") {
		v, err := strconv.ParseFloat(p.Get("ores_prob_to"), 64)
		if err != nil {
			v = 1
		}
		d.oresProbTo = &v
	}
	return d, nil
}

func optInt(p Params, key string) *int {
	if n, ok := p.Int(key); ok {
		return &n
	}
	return nil
}

// parseDepth reads a category depth; negative means unlimited.
func parseDepth(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	if n < 0 {
		return 999
	}
	return n
}

// normalizeTitles upper-cases the first letter unless caseSensitive.
func normalizeTitles(titles []string, caseSensitive bool) []string {
	ret := make([]string, len(titles))
	for i, t := range titles {
		if !caseSensitive {
			t = wiki.SpacesToUnderscores(wiki.UcFirst(wiki.UnderscoresToSpaces(t)))
		}
		ret[i] = t
	}
	return ret
}

func (d *databaseQuery) gatherLinkCount() bool {
	return d.minLinks != nil || d.maxLinks != nil
}

// primary is what drives the base query.
type primary int

const (
	primaryCategories primary = iota
	primaryTemplates
	primaryLinks
	primaryPageList
	primaryNoWikidata
	primaryCreatedBy
)

func (d *databaseQuery) primary(input *PageList) (primary, error) {
	switch {
	case len(d.categoryTreesPos) > 0:
		return primaryCategories, nil
	case len(d.templatesYes) > 0 || len(d.templatesAny) > 0:
		return primaryTemplates, nil
	case len(d.linkedFromAll) > 0 || len(d.linkedFromAny) > 0 || len(d.linksToAll) > 0 || len(d.linksToAny) > 0:
		return primaryLinks, nil
	case input != nil:
		return primaryPageList, nil
	case d.wikidataItem == "without":
		return primaryNoWikidata, nil
	case len(d.createdBy) > 0:
		return primaryCreatedBy, nil
	}
	return 0, invalidf("no categories, templates, links or other primary database source given")
}

// pages runs the query. With input, it keeps those pages of input that pass
// the filters, instead of starting from categories etc.
func (d *databaseQuery) pages(ctx context.Context, input *PageList) (*PageList, error) {
	if (len(d.catPos) > 0 || len(d.catNeg) > 0) && d.skipTrackingCats {
		name, err := d.resolveTrackingCategory(ctx)
		if err != nil {
			return nil, err
		}
		d.trackingCategory = name
	}
	var err error
	if d.categoryTreesPos, err = d.categoryTrees(ctx, d.catPos); err != nil {
		return nil, err
	}
	if d.categoryTreesNeg, err = d.categoryTrees(ctx, d.catNeg); err != nil {
		return nil, err
	}

	prim, err := d.primary(input)
	if err != nil {
		return nil, err
	}
	filters := d.filters(prim)
	baseCluster := filters.baseCluster(d.wiki, d.baseTables(prim))

	var seed []int64
	if prim != primaryPageList {
		if seed, err = d.fetchSeed(ctx, filters, baseCluster); err != nil {
			return nil, err
		}
	}
	plan := basePlan{filters: filters, cluster: baseCluster, seed: seed}

	var queries []*replica.Query
	switch prim {
	case primaryCategories:
		groups := d.categoryTreesPos
		if d.combineUnion {
			// Any category will do: one merged group.
			var all []string
			for _, g := range groups {
				all = append(all, g...)
			}
			slices.Sort(all)
			groups = [][]string{slices.Compact(all)}
		}
		// Keep each statement within the placeholder limit, next to the
		// seed and the filters.
		budget := maxPlaceholders - min(len(seed), pageBatchSize) - filters.args()
		for _, batch := range categoryBatches(groups, budget) {
			for _, ids := range plan.seedChunks() {
				queries = append(queries, d.categoryQuery(baseCluster, batch, ids))
			}
		}
	case primaryPageList:
		for _, b := range input.batches(pageBatchSize) {
			queries = append(queries, d.selectPages(baseCluster).Add(" FROM page p WHERE ").Append(b.whereP()))
		}
	default:
		for _, ids := range plan.seedChunks() {
			q := d.selectPages(baseCluster).Add(" FROM page p WHERE 1=1")
			if ids != nil {
				q.Add(" AND p.page_id").In(replica.Ints(ids))
			}
			queries = append(queries, q)
		}
	}

	result := NewPageList(d.wiki)
	for _, q := range queries {
		pl, err := d.runBaseQuery(ctx, plan, q)
		if err != nil {
			return nil, err
		}
		result.Union(pl)
		if err := result.checkLimit(d.run.env.MaxResults); err != nil {
			return nil, err
		}
	}
	if err := d.subtractNegativeCategories(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

// basePlan is how the base queries are run.
type basePlan struct {
	filters filters
	cluster replica.Cluster
	seed    []int64 // nil: no seed
}

// seedChunks returns the page ID restrictions to run the base query under:
// one per chunk of the seed, or a single unrestricted (nil) run.
func (p basePlan) seedChunks() [][]int64 {
	if p.seed == nil {
		return [][]int64{nil}
	}
	if len(p.seed) == 0 {
		return nil // the seed matches nothing, so neither does the query
	}
	return chunks(p.seed, pageBatchSize)
}

// baseTables are the tables the base query joins itself besides page.
func (d *databaseQuery) baseTables(prim primary) []string {
	var tables []string
	if prim == primaryCategories {
		tables = append(tables, "categorylinks", "linktarget")
	}
	if d.gatherLinkCount() {
		tables = append(tables, "pagelinks")
	}
	return tables
}

// selectPages starts the base query. page_touched comes from revision, which
// only the core cluster has; elsewhere it is filled in by the deferred pass.
func (d *databaseQuery) selectPages(cluster replica.Cluster) *replica.Query {
	q := replica.New("SELECT DISTINCT p.page_id,p.page_title,p.page_namespace,")
	if cluster == replica.Core {
		q.Add("(SELECT rev_timestamp FROM revision WHERE rev_id=p.page_latest LIMIT 1)")
	} else {
		q.Add("''")
	}
	q.Add(" AS page_touched,p.page_len")
	if d.gatherLinkCount() {
		q.Add(",(SELECT count(*) FROM pagelinks WHERE pl_from=p.page_id) AS link_count")
	} else {
		q.Add(",0 AS link_count")
	}
	return q
}

// categoryQuery selects the pages in all of the category groups of batch
// (for union mode, there is a single merged group). A seed restricts the members considered.
func (d *databaseQuery) categoryQuery(cluster replica.Cluster, batch [][]string, seed []int64) *replica.Query {
	first := batch[0]
	q := d.selectPages(cluster).Add(" FROM (SELECT cl_from FROM categorylinks,linktarget WHERE ")
	if seed != nil {
		q.Add("cl_from").In(replica.Ints(seed)).Add(" AND ")
	}
	q.Add("lt_id=cl_target_id AND lt_namespace=14 AND lt_title").In(replica.Strings(first)).
		Add(") cl0 INNER JOIN page p ON p.page_id=cl0.cl_from WHERE 1=1")
	for _, group := range batch[1:] {
		q.Add(" AND EXISTS (SELECT 1 FROM categorylinks cl,linktarget lt " +
			"WHERE cl.cl_from=p.page_id AND lt.lt_id=cl.cl_target_id AND lt.lt_namespace=14 AND lt.lt_title").
			In(replica.Strings(group)).Add(")")
	}
	return q
}

// categoryBatches splits each category group into chunks and returns their
// cross product, with at most budget category names per batch.
func categoryBatches(groups [][]string, budget int) [][][]string {
	size := max(1, min(categoryBatchSize, budget/len(groups)))
	ret := [][][]string{{}}
	for _, group := range groups {
		var next [][][]string
		for _, prefix := range ret {
			for _, chunk := range chunks(group, size) {
				next = append(next, append(slices.Clone(prefix), chunk))
			}
		}
		ret = next
	}
	return ret
}

// runBaseQuery runs one base query, then the deferred filters.
func (d *databaseQuery) runBaseQuery(ctx context.Context, plan basePlan, q *replica.Query) (*PageList, error) {
	inline, deferred, err := plan.filters.split(d.wiki, plan.cluster)
	if err != nil {
		return nil, err
	}
	q.Append(inline)
	d.addHaving(q)
	limit := d.run.env.MaxResults
	// One row past the limit tells "too many" from "exactly the limit".
	q.Add(" LIMIT ?", limit+1)

	var rows []pageRow
	err = d.run.query(ctx, d.wiki, plan.cluster, q, func(rs *sql.Rows) error {
		var row pageRow
		if err := rs.Scan(&row.id, &row.title, &row.ns, &row.touched, &row.len, &row.linkCount); err != nil {
			return err
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(rows) > limit {
		return nil, tooManyResults("this query yields", limit)
	}
	if rows, err = d.applyDeferred(ctx, plan.cluster, deferred, rows); err != nil {
		return nil, err
	}

	pl := NewPageList(d.wiki)
	for _, row := range rows {
		e := &Entry{
			Title:   wiki.NewTitle(row.title, row.ns),
			PageID:  row.id,
			Len:     row.len,
			Touched: row.touched.String,
		}
		if d.gatherLinkCount() {
			n := int(row.linkCount)
			e.LinkCount = &n
		}
		pl.Add(e)
	}
	return pl, nil
}

type pageRow struct {
	id        int64
	title     string
	ns        int
	touched   sql.NullString
	len       int64
	linkCount int64
}

// addHaving filters on the link count.
func (d *databaseQuery) addHaving(q *replica.Query) {
	switch {
	case d.minLinks != nil && d.maxLinks != nil:
		q.Add(" HAVING link_count>=? AND link_count<=?", *d.minLinks, *d.maxLinks)
	case d.minLinks != nil:
		q.Add(" HAVING link_count>=?", *d.minLinks)
	case d.maxLinks != nil:
		q.Add(" HAVING link_count<=?", *d.maxLinks)
	}
}

// applyDeferred applies the filter clauses the base query's cluster could
// not serve, one pass per cluster, and fills in page_touched where the base
// query could not select it.
func (d *databaseQuery) applyDeferred(ctx context.Context, base replica.Cluster, deferred map[replica.Cluster]*replica.Query, rows []pageRow) ([]pageRow, error) {
	fetchTouched := base != replica.Core
	if fetchTouched && deferred[replica.Core] == nil {
		deferred[replica.Core] = new(replica.Query)
	}
	for _, cluster := range []replica.Cluster{replica.Core, replica.Links, replica.TermStore} {
		clauses, ok := deferred[cluster]
		if !ok || len(rows) == 0 {
			continue
		}
		wantTouched := fetchTouched && cluster == replica.Core
		ids := make([]int64, len(rows))
		for i, row := range rows {
			ids[i] = row.id
		}
		var queries []*replica.Query
		for _, chunk := range chunks(ids, pageBatchSize) {
			q := replica.New("SELECT p.page_id,")
			if wantTouched {
				q.Add("(SELECT rev_timestamp FROM revision WHERE rev_id=p.page_latest LIMIT 1)")
			} else {
				q.Add("''")
			}
			q.Add(" FROM page p WHERE p.page_id").In(replica.Ints(chunk)).Append(clauses)
			queries = append(queries, q)
		}
		kept := map[int64]sql.NullString{}
		err := d.run.queryAll(ctx, d.wiki, cluster, queries, func(rs *sql.Rows) error {
			var id int64
			var touched sql.NullString
			if err := rs.Scan(&id, &touched); err != nil {
				return err
			}
			kept[id] = touched
			return nil
		})
		if err != nil {
			return nil, err
		}
		rows = slices.DeleteFunc(rows, func(row pageRow) bool {
			_, ok := kept[row.id]
			return !ok
		})
		if wantTouched {
			for i := range rows {
				rows[i].touched = kept[rows[i].id]
			}
		}
	}
	return rows, nil
}

// fetchSeed returns the page IDs the base query is restricted to, if a
// deferred filter offers a seed and it names at most maxSeedPages pages.
func (d *databaseQuery) fetchSeed(ctx context.Context, f filters, base replica.Cluster) ([]int64, error) {
	cluster, q, ok, err := f.seed(d.wiki, base)
	if err != nil || !ok {
		return nil, err
	}
	q = q.Clone().Add(" LIMIT ?", maxSeedPages+1)
	ids := []int64{}
	err = d.run.query(ctx, d.wiki, cluster, q, func(rows *sql.Rows) error {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(ids) > maxSeedPages {
		return nil, nil
	}
	slices.Sort(ids)
	return ids, nil
}

// subtractNegativeCategories removes the members of the negative category
// trees. It is done in memory rather than in SQL so that a deep tree cannot
// exceed the placeholder limit.
func (d *databaseQuery) subtractNegativeCategories(ctx context.Context, result *PageList) error {
	if len(d.categoryTreesNeg) == 0 || result.Len() == 0 {
		return nil
	}
	var cats []string
	for _, tree := range d.categoryTreesNeg {
		cats = append(cats, tree...)
	}
	slices.Sort(cats)
	cats = slices.Compact(cats)

	limit := d.run.env.MaxResults
	var queries []*replica.Query
	for _, chunk := range chunks(cats, categoryBatchSize) {
		queries = append(queries, replica.New(
			"SELECT DISTINCT p.page_title,p.page_namespace FROM page p,categorylinks,linktarget "+
				"WHERE p.page_id=cl_from AND lt_id=cl_target_id AND lt_namespace=14 AND lt_title",
		).In(replica.Strings(chunk)).Add(" LIMIT ?", limit+1))
	}
	cluster, err := replica.ClusterFor(d.wiki, tablesCategoryLinks...)
	if err != nil {
		return err
	}
	excluded := NewPageList(d.wiki)
	err = d.run.queryAll(ctx, d.wiki, cluster, queries, func(rows *sql.Rows) error {
		var title string
		var ns int
		if err := rows.Scan(&title, &ns); err != nil {
			return err
		}
		excluded.AddTitle(wiki.NewTitle(title, ns))
		if excluded.Len() > limit {
			return tooManyResults("the categories to exclude hold", limit)
		}
		return nil
	})
	if err != nil {
		return err
	}
	result.Subtract(excluded)
	return nil
}

// categoryTrees expands each "Name" or "Name|depth" to its category tree.
func (d *databaseQuery) categoryTrees(ctx context.Context, cats []string) ([][]string, error) {
	var ret [][]string
	for _, c := range cats {
		name, depthStr, hasDepth := strings.Cut(c, "|")
		depth := d.depth
		if hasDepth {
			n, err := strconv.Atoi(depthStr)
			if err != nil {
				continue
			}
			depth = parseDepth(strconv.Itoa(n))
		}
		tree, err := d.categoryTree(ctx, name, depth)
		if err != nil {
			return nil, err
		}
		if len(tree) > 0 {
			ret = append(ret, tree)
		}
	}
	return ret, nil
}

// categoryTree returns root and its subcategories up to depth, with
// underscores. Depth 0 is the root only.
func (d *databaseQuery) categoryTree(ctx context.Context, root string, depth int) ([]string, error) {
	norm := func(s string) string {
		s = wiki.UnderscoresToSpaces(s)
		if !d.categoryCaseSensitive {
			s = wiki.UcFirst(s)
		}
		return wiki.SpacesToUnderscores(s)
	}
	root = norm(root)
	done := map[string]bool{root: true}
	todo := []string{root}
	// Where page_props cannot be joined with categorylinks (Commons), hidden
	// categories are looked up in a second query.
	filterHiddenInline := d.skipHiddenCats && clusterHostsAll(d.wiki, append(slices.Clone(tablesCategoryLinks), "page_props"))
	cluster, err := replica.ClusterFor(d.wiki, tablesCategoryLinks...)
	if err != nil {
		return nil, err
	}
	for level := 0; level < depth && len(todo) > 0; level++ {
		var found []string
		var queries []*replica.Query
		for _, chunk := range chunks(todo, categoryBatchSize) {
			queries = append(queries, d.subcategoriesQuery(chunk, filterHiddenInline))
		}
		type subcat struct {
			id    int64
			title string
		}
		var subcats []subcat
		err := d.run.queryAll(ctx, d.wiki, cluster, queries, func(rows *sql.Rows) error {
			var s subcat
			if err := rows.Scan(&s.id, &s.title); err != nil {
				return err
			}
			subcats = append(subcats, s)
			return nil
		})
		if err != nil {
			return nil, err
		}
		if d.skipHiddenCats && !filterHiddenInline && len(subcats) > 0 {
			ids := make([]int64, len(subcats))
			for i, s := range subcats {
				ids[i] = s.id
			}
			hidden, err := d.hiddenCategories(ctx, ids)
			if err != nil {
				return nil, err
			}
			subcats = slices.DeleteFunc(subcats, func(s subcat) bool { return hidden[s.id] })
		}
		for _, s := range subcats {
			if t := norm(s.title); !done[t] {
				done[t] = true
				found = append(found, t)
			}
		}
		if len(done) > maxSubcategories {
			return nil, invalidf("subcategories of %q exceed %d, please limit the category depth, or fix the category tree", root, maxSubcategories)
		}
		todo = found
	}
	ret := make([]string, 0, len(done))
	for t := range done {
		ret = append(ret, t)
	}
	slices.Sort(ret)
	return ret, nil
}

// clusterHostsAll reports whether one cluster can serve all tables.
func clusterHostsAll(w string, tables []string) bool {
	_, err := replica.ClusterFor(w, tables...)
	return err == nil
}

// subcategoriesQuery selects the direct subcategories of categories. The
// optional filters act on discovered subcategories only, never on the roots.
func (d *databaseQuery) subcategoriesQuery(categories []string, filterHidden bool) *replica.Query {
	q := replica.New("SELECT DISTINCT page_id,page_title FROM page,categorylinks,linktarget " +
		"WHERE lt_id=cl_target_id AND cl_from=page_id AND cl_type='subcat' AND lt_namespace=14 AND lt_title").
		In(replica.Strings(categories))
	if filterHidden {
		q.Add(" AND NOT EXISTS (SELECT 1 FROM page_props WHERE pp_page=page_id AND pp_propname='hiddencat')")
	}
	if d.trackingCategory != "" {
		q.Add(" AND NOT EXISTS (SELECT 1 FROM categorylinks cltc,linktarget lttc "+
			"WHERE cltc.cl_from=page_id AND lttc.lt_id=cltc.cl_target_id AND lttc.lt_namespace=14 AND lttc.lt_title=?)",
			d.trackingCategory)
	}
	return q
}

// hiddenCategories returns which of ids are __HIDDENCAT__ categories, for
// wikis where page_props cannot be joined with categorylinks.
func (d *databaseQuery) hiddenCategories(ctx context.Context, ids []int64) (map[int64]bool, error) {
	hidden := map[int64]bool{}
	var queries []*replica.Query
	for _, chunk := range chunks(ids, pageBatchSize) {
		queries = append(queries, replica.New("SELECT pp_page FROM page_props WHERE pp_propname='hiddencat' AND pp_page").
			In(replica.Ints(chunk)))
	}
	err := d.run.queryAll(ctx, d.wiki, replica.Core, queries, func(rows *sql.Rows) error {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		hidden[id] = true
		return nil
	})
	return hidden, err
}

// resolveTrackingCategory returns the local name of the container of
// tracking categories, via the sitelink of Q6964088. Without one, the
// requested filter cannot be applied, which is an error.
func (d *databaseQuery) resolveTrackingCategory(ctx context.Context) (string, error) {
	var page string
	q := replica.New("SELECT ips_site_page FROM wb_items_per_site WHERE ips_item_id=? AND ips_site_id=?", trackingCategoriesItem, d.wiki)
	err := d.run.query(ctx, wikidataWiki, replica.Core, q, func(rows *sql.Rows) error {
		return rows.Scan(&page)
	})
	if err != nil {
		return "", err
	}
	if page == "" {
		return "", invalidf("cannot skip tracking categories on %s: no local category is linked to wikidata:Q%d", d.wiki, trackingCategoriesItem)
	}
	t := d.site.ParseTitle(page)
	if t.NS != 14 {
		return "", invalidf("cannot skip tracking categories on %s: the wikidata:Q%d sitelink %q is not a category", d.wiki, trackingCategoriesItem, page)
	}
	return t.Underscored(), nil
}

// maxAgeAfter returns the timestamp max_age hours ago.
func maxAgeAfter(hours int, now time.Time) string {
	return now.UTC().Add(-time.Duration(hours) * time.Hour).Format("20060102150405")
}
