package petscan

import (
	"slices"
	"time"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// filter is one clause of the base query, " AND …" over the page alias p,
// plus the tables it reads besides page. It reads the same in the base query
// as in a deferred "SELECT p.page_id FROM page p WHERE …" pass.
type filter struct {
	tables []string
	sql    *replica.Query
	// seed, if set, selects a superset of the page IDs the clause matches,
	// driven by an index of its own. Only clauses that typically match few
	// pages offer one: when such a clause is deferred, the base query can
	// start from the few pages instead of, say, a large category tree.
	seed *replica.Query
}

type filters []filter

func (f *filters) add(tables []string, sql *replica.Query) {
	f.addSeeded(tables, sql, nil)
}

// args returns the number of bound values of all clauses.
func (f filters) args() int {
	n := 0
	for _, fl := range f {
		n += len(fl.sql.Args())
	}
	return n
}

func (f *filters) addSeeded(tables []string, sql *replica.Query, seed *replica.Query) {
	*f = append(*f, filter{tables: tables, sql: sql, seed: seed})
}

// baseCluster returns the cluster to run the base query on. The links
// cluster wins as soon as anything needs it: on Commons the links tables
// hold what a query selects on — a category, a template, a link — while the
// core-only filters only narrow an already small set.
func (f filters) baseCluster(w string, baseTables []string) replica.Cluster {
	tables := slices.Clone(baseTables)
	for _, fl := range f {
		tables = append(tables, fl.tables...)
	}
	for _, table := range tables {
		if !replica.Core.Hosts(w, table) && replica.Links.AppliesTo(w) {
			return replica.Links
		}
	}
	return replica.Core
}

// seed returns the seed of the first clause a base query on base has to
// defer, and the cluster to run it on.
func (f filters) seed(w string, base replica.Cluster) (replica.Cluster, *replica.Query, bool, error) {
	for _, fl := range f {
		if base.Hosts(w, fl.tables...) || fl.seed == nil {
			continue
		}
		cluster, err := replica.ClusterFor(w, fl.tables...)
		return cluster, fl.seed, err == nil, err
	}
	return replica.Core, nil, false, nil
}

// split returns the clauses a base query on base can carry, and the others
// grouped by the cluster that can serve them.
func (f filters) split(w string, base replica.Cluster) (*replica.Query, map[replica.Cluster]*replica.Query, error) {
	inline := new(replica.Query)
	deferred := map[replica.Cluster]*replica.Query{}
	for _, fl := range f {
		if base.Hosts(w, fl.tables...) {
			inline.Append(fl.sql)
			continue
		}
		cluster, err := replica.ClusterFor(w, append([]string{"page"}, fl.tables...)...)
		if err != nil {
			return nil, nil, err
		}
		if deferred[cluster] == nil {
			deferred[cluster] = new(replica.Query)
		}
		deferred[cluster].Append(fl.sql)
	}
	return inline, deferred, nil
}

// filters collects every clause the parameters ask for. Negative categories
// are not among them; see subtractNegativeCategories.
func (d *databaseQuery) filters(prim primary) filters {
	var f filters
	if len(d.namespaces) > 0 && prim != primaryPageList {
		f.add(nil, replica.New(" AND p.page_namespace").In(replica.Ints(d.namespaces)))
	}
	d.templateFilters(&f)
	d.linkFilters(&f)
	d.pagePropertyFilters(&f)
	d.oresFilter(&f)
	d.lastEditFilters(&f)
	d.createdByFilter(&f)
	d.sizeFilters(&f)
	d.lastEditedFilter(&f)
	return f
}

func (d *databaseQuery) templateFilters(f *filters) {
	for _, t := range d.templatesYes {
		f.add(tablesTemplateLinks, templateClause([]string{t}, d.templatesYesTalk, false))
	}
	if len(d.templatesAny) > 0 {
		f.add(tablesTemplateLinks, templateClause(d.templatesAny, d.templatesAnyTalk, false))
	}
	if len(d.templatesNo) > 0 {
		f.add(tablesTemplateLinks, templateClause(d.templatesNo, d.templatesNoTalk, true))
	}
}

// templateClause matches pages that (or, with negate, that don't) use any of
// templates — or whose talk page does, with onTalkPage.
func templateClause(templates []string, onTalkPage, negate bool) *replica.Query {
	q := new(replica.Query)
	if negate {
		q.Add(" AND NOT EXISTS ")
	} else {
		q.Add(" AND EXISTS ")
	}
	if onTalkPage {
		q.Add("(SELECT 1 FROM page pt,templatelinks,linktarget " +
			"WHERE pt.page_namespace=p.page_namespace+1 AND pt.page_title=p.page_title " +
			"AND tl_from=pt.page_id AND tl_target_id=lt_id AND lt_namespace=10 AND lt_title")
	} else {
		q.Add("(SELECT 1 FROM templatelinks,linktarget " +
			"WHERE tl_from=p.page_id AND tl_target_id=lt_id AND lt_namespace=10 AND lt_title")
	}
	return q.In(replica.Strings(templates)).Add(")")
}

func (d *databaseQuery) linkFilters(f *filters) {
	add := func(titles []string, linksTo, negate bool) {
		q := new(replica.Query)
		if negate {
			q.Add(" AND p.page_id NOT IN (")
		} else {
			q.Add(" AND p.page_id IN (")
		}
		if linksTo {
			// Pages linking to the given ones.
			q.Add("SELECT pl_from FROM pagelinks,linktarget WHERE pl_target_id=lt_id AND (")
		} else {
			// Pages the given ones link to.
			q.Add("SELECT p_to.page_id FROM page p_to,page p_from,pagelinks,linktarget " +
				"WHERE pl_target_id=lt_id AND p_from.page_id=pl_from " +
				"AND lt_namespace=p_to.page_namespace AND lt_title=p_to.page_title AND (")
		}
		// One subquery with OR'ed namespace groups: MariaDB cannot turn an IN
		// over a UNION into a semi-join.
		for i, b := range d.titlesByNamespace(titles) {
			if i > 0 {
				q.Add(" OR ")
			}
			if linksTo {
				q.Add("(lt_namespace=? AND lt_title", b.NS)
			} else {
				q.Add("(p_from.page_namespace=? AND p_from.page_title", b.NS)
			}
			q.In(replica.Strings(b.Titles)).Add(")")
		}
		f.add(tablesPageLinks, q.Add("))"))
	}
	for _, t := range d.linksToAll {
		add([]string{t}, true, false)
	}
	if len(d.linksToAny) > 0 {
		add(d.linksToAny, true, false)
	}
	if len(d.linksToNone) > 0 {
		add(d.linksToNone, true, true)
	}
	for _, t := range d.linkedFromAll {
		add([]string{t}, false, false)
	}
	if len(d.linkedFromAny) > 0 {
		add(d.linkedFromAny, false, false)
	}
	if len(d.linkedFromNone) > 0 {
		add(d.linkedFromNone, false, true)
	}
}

// titlesByNamespace parses prefixed titles and groups them by namespace.
func (d *databaseQuery) titlesByNamespace(titles []string) []titleBatch {
	pl := NewPageList(d.wiki)
	for _, t := range titles {
		pl.AddTitle(d.site.ParseTitle(t))
	}
	return pl.batches(len(titles))
}

func (d *databaseQuery) pagePropertyFilters(f *filters) {
	switch d.pageImage {
	case "yes":
		f.add(tablesPageProps, replica.New(" AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=p.page_id AND pp_propname IN ('page_image','page_image_free'))"))
	case "free":
		f.add(tablesPageProps, replica.New(" AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=p.page_id AND pp_propname='page_image_free')"))
	case "nonfree":
		f.add(tablesPageProps, replica.New(" AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=p.page_id AND pp_propname='page_image')"))
	case "no":
		f.add(tablesPageProps, replica.New(" AND NOT EXISTS (SELECT 1 FROM page_props WHERE pp_page=p.page_id AND pp_propname IN ('page_image','page_image_free'))"))
	}

	if d.softRedirects == "yes" {
		// TODO: the template's name differs per wiki; Wikidata Q4844001.
		f.add(tablesTemplateLinks, replica.New(" AND EXISTS (SELECT 1 FROM templatelinks,linktarget "+
			"WHERE tl_from=p.page_id AND tl_target_id=lt_id AND lt_namespace=10 AND lt_title=?)", "Soft_redirect"))
	}
	switch d.redirects {
	case "yes":
		f.add(nil, replica.New(" AND p.page_is_redirect=1"))
	case "no":
		f.add(nil, replica.New(" AND p.page_is_redirect=0"))
	}
	switch d.disambiguation {
	case "yes":
		f.add(tablesPageProps, replica.New(" AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=p.page_id AND pp_propname='disambiguation')"))
	case "no":
		f.add(tablesPageProps, replica.New(" AND NOT EXISTS (SELECT 1 FROM page_props WHERE pp_page=p.page_id AND pp_propname='disambiguation')"))
	}
	switch d.talkPageExists {
	case "yes":
		f.add(nil, replica.New(" AND EXISTS (SELECT 1 FROM page talk_p WHERE talk_p.page_title=p.page_title AND talk_p.page_namespace=p.page_namespace+1)"))
	case "no":
		f.add(nil, replica.New(" AND NOT EXISTS (SELECT 1 FROM page talk_p WHERE talk_p.page_title=p.page_title AND talk_p.page_namespace=p.page_namespace+1)"))
	}
	if d.wikidataItem == "without" {
		f.add(tablesPageProps, replica.New(" AND NOT EXISTS (SELECT 1 FROM page_props WHERE pp_page=p.page_id AND pp_propname='wikibase_item')"))
	}
}

func (d *databaseQuery) oresFilter(f *filters) {
	if d.oresType == "" || d.oresType == "any" ||
		(d.oresPrediction == "any" && d.oresProbFrom == nil && d.oresProbTo == nil) {
		return
	}
	q := replica.New(" AND EXISTS (SELECT 1 FROM ores_classification WHERE oresc_rev=p.page_latest "+
		"AND oresc_model IN (SELECT oresm_id FROM ores_model WHERE oresm_is_current=1 AND oresm_name=?)", d.oresType)
	switch d.oresPrediction {
	case "yes":
		q.Add(" AND oresc_is_predicted=1")
	case "no":
		q.Add(" AND oresc_is_predicted=0")
	}
	if d.oresProbFrom != nil {
		q.Add(" AND oresc_probability>=?", *d.oresProbFrom)
	}
	if d.oresProbTo != nil {
		q.Add(" AND oresc_probability<=?", *d.oresProbTo)
	}
	f.add(tablesORES, q.Add(")"))
}

func (d *databaseQuery) lastEditFilters(f *filters) {
	switch d.lastEditAnon {
	case "yes":
		f.add(tablesRevisionActor, replica.New(" AND EXISTS (SELECT 1 FROM revision,actor WHERE rev_id=p.page_latest AND rev_actor=actor_id AND actor_user IS NULL)"))
	case "no":
		f.add(tablesRevisionActor, replica.New(" AND EXISTS (SELECT 1 FROM revision,actor WHERE rev_id=p.page_latest AND rev_actor=actor_id AND actor_user IS NOT NULL)"))
	}
	switch d.lastEditBot {
	case "yes":
		f.add(tablesRevisionBots, replica.New(" AND EXISTS (SELECT 1 FROM revision,actor,user_groups WHERE rev_id=p.page_latest AND rev_actor=actor_id AND actor_user=ug_user AND ug_group='bot')"))
	case "no":
		f.add(tablesRevisionBots, replica.New(" AND NOT EXISTS (SELECT 1 FROM revision,actor,user_groups WHERE rev_id=p.page_latest AND rev_actor=actor_id AND actor_user=ug_user AND ug_group='bot')"))
	}
	// fp_pending_since is NULL when a page has no unreviewed changes, so
	// "last edit flagged" means no pending changes. Pages not enrolled in
	// FlaggedRevs count as flagged, as in PetScan.
	switch d.lastEditFlagged {
	case "yes":
		f.add(tablesFlaggedPages, replica.New(" AND NOT EXISTS (SELECT 1 FROM flaggedpages WHERE fp_page_id=p.page_id AND fp_pending_since IS NOT NULL)"))
	case "no":
		f.add(tablesFlaggedPages, replica.New(" AND EXISTS (SELECT 1 FROM flaggedpages WHERE fp_page_id=p.page_id AND fp_pending_since IS NOT NULL)"))
	}
}

// createdByFilter keeps pages created by any of the given users. A user's
// creations are few and indexed by actor, so the sub-select doubles as seed.
func (d *databaseQuery) createdByFilter(f *filters) {
	if len(d.createdBy) == 0 {
		return
	}
	creations := replica.New("SELECT r.rev_page FROM revision r INNER JOIN actor a ON r.rev_actor=a.actor_id " +
		"WHERE r.rev_parent_id=0 AND a.actor_name").In(replica.Strings(spaces(d.createdBy)))
	clause := replica.New(" AND p.page_id IN (").Append(creations).Add(")")
	f.addSeeded(tablesRevisionActor, clause, creations)
}

// spaces replaces underscores by spaces, as user names are stored.
func spaces(names []string) []string {
	ret := make([]string, len(names))
	for i, n := range names {
		ret[i] = wiki.UnderscoresToSpaces(n)
	}
	return ret
}

func (d *databaseQuery) sizeFilters(f *filters) {
	if d.larger != nil {
		f.add(nil, replica.New(" AND p.page_len>=?", *d.larger))
	}
	if d.smaller != nil {
		f.add(nil, replica.New(" AND p.page_len<=?", *d.smaller))
	}
	if d.sinceRev0 != nil {
		f.add(tablesRevision, replica.New(" AND p.page_len<=(SELECT rev_len FROM revision WHERE rev_page=p.page_id AND rev_parent_id=0 LIMIT 1)*?/100", *d.sinceRev0))
	}
}

// lastEditedFilter keeps pages last edited (or, with only_new, created)
// within before/after; max_age overrides both.
func (d *databaseQuery) lastEditedFilter(f *filters) {
	before, after := d.before, d.after
	if d.maxAgeHours != nil {
		before, after = "", maxAgeAfter(*d.maxAgeHours, time.Now())
	}
	if before == "" && after == "" {
		return
	}
	q := replica.New(" AND EXISTS (SELECT 1 FROM revision r WHERE r.rev_page=p.page_id")
	if d.onlyNew {
		q.Add(" AND r.rev_parent_id=0")
	} else {
		q.Add(" AND r.rev_id=p.page_latest")
	}
	if before != "" {
		q.Add(" AND r.rev_timestamp<=?", before)
	}
	if after != "" {
		q.Add(" AND r.rev_timestamp>=?", after)
	}
	q.Add(")")

	// A lower bound on the timestamp is selective enough to drive a query
	// off revision's timestamp index; pages with any revision in the window
	// are a superset of the clause's matches.
	var seed *replica.Query
	if after != "" {
		seed = replica.New("SELECT DISTINCT rev_page FROM revision WHERE rev_timestamp>=?", after)
		if before != "" {
			seed.Add(" AND rev_timestamp<=?", before)
		}
		if d.onlyNew {
			seed.Add(" AND rev_parent_id=0")
		}
	}
	f.addSeeded(tablesRevision, q, seed)
}
