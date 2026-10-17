package petscan

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// postProcess filters and annotates the combined result, in PetScan's order.
func (r *Run) postProcess(ctx context.Context, sourceNames []string) error {
	result := r.result
	steps := []struct {
		name string
		fn   func(context.Context, *PageList) error
	}{
		{"wikidata filters", r.filterWikidata},
		{"sitelinks", func(ctx context.Context, pl *PageList) error {
			if slices.Equal(sourceNames, []string{"sitelinks"}) {
				return nil // already applied by the source
			}
			return r.filterSitelinks(ctx, pl)
		}},
		{"labels", r.filterLabels},
		{"common wiki", r.convertToCommonWiki},
		{"database filters", func(ctx context.Context, pl *PageList) error {
			if slices.Contains(sourceNames, "categories") {
				return nil // already applied by the source
			}
			return r.applyDatabaseFilters(ctx, pl)
		}},
		{"wikidata item", r.filterByWikidataItem},
		{"files", r.addFileData},
		{"page properties", r.addPageProperties},
		{"namespace conversion", r.convertNamespaces},
		{"subpages", r.processSubpages},
		{"wikidata annotation", r.annotateWikidataItems},
		{"metadata", func(ctx context.Context, pl *PageList) error {
			language := r.params.GetDefault("wikidata_label_language", r.params.GetDefault("interface_language", "en"))
			return r.loadMissingMetadata(ctx, pl, language)
		}},
		{"regexp filter", r.filterRegexp},
		{"search filter", r.filterSearch},
		{"redlinks", r.processRedlinks},
	}
	for _, step := range steps {
		if err := step.fn(ctx, result); err != nil {
			return err
		}
	}
	return nil
}

// convertToCommonWiki converts the result to the wiki the common_wiki
// parameter asks for.
func (r *Run) convertToCommonWiki(ctx context.Context, pl *PageList) error {
	var target string
	sourceWiki := func(source string) (string, error) {
		if w, ok := r.wikiBySource[source]; ok {
			return w, nil
		}
		return "", invalidf("%s wiki requested as output, but not set", source)
	}
	var err error
	switch mode := r.params.GetDefault("common_wiki", "auto"); mode {
	case "auto":
		return nil
	case "wikidata":
		target = wikidataWiki
	case "cats":
		target, err = sourceWiki("categories")
	case "pagepile":
		target, err = sourceWiki("pagepile")
	case "manual":
		target, err = sourceWiki("manual")
		if err != nil && r.params.Has("common_wiki_other") {
			target, err = r.params.Get("common_wiki_other"), nil
		}
	case "other":
		if target = r.params.Get("common_wiki_other"); target == "" {
			err = invalidf("other wiki for output expected, but not given in text field")
		}
	default:
		err = invalidf("unknown output wiki type %q", mode)
	}
	if err != nil {
		return err
	}
	return r.convertToWiki(ctx, pl, target)
}

// applyDatabaseFilters applies the database filters (templates, links, page
// properties, …) to a result that did not come from the categories source.
func (r *Run) applyDatabaseFilters(ctx context.Context, pl *PageList) error {
	d, err := newDatabaseQueryFor(ctx, r, pl.Wiki)
	if err != nil {
		return err
	}
	filtered, err := d.pages(ctx, pl)
	if err != nil {
		return err
	}
	pl.Replace(filtered)
	return nil
}

// filterByWikidataItem keeps pages with (or without) a Wikidata item.
func (r *Run) filterByWikidataItem(ctx context.Context, pl *PageList) error {
	mode := r.params.GetDefault("wikidata_item", "no")
	if pl.IsWikidata() || (mode != "any" && mode != "with" && mode != "without") {
		return nil
	}
	if err := r.annotateWikidataItems(ctx, pl); err != nil {
		return err
	}
	switch mode {
	case "with":
		pl.Retain(func(e *Entry) bool { return e.WikidataItem != "" })
	case "without":
		pl.Retain(func(e *Entry) bool { return e.WikidataItem == "" })
	}
	return nil
}

// annotateWikidataItems adds the Wikidata item of each page.
func (r *Run) annotateWikidataItems(ctx context.Context, pl *PageList) error {
	if pl.IsWikidata() || pl.Len() == 0 {
		return nil
	}
	var queries []*replica.Query
	for _, b := range pl.batches(pageBatchSize) {
		queries = append(queries, replica.New(
			"SELECT page_title,page_namespace,pp_value FROM page,page_props "+
				"WHERE pp_page=page_id AND pp_propname='wikibase_item' AND ",
		).Append(b.where()))
	}
	return r.queryAll(ctx, pl.Wiki, replica.Core, queries, func(rows *sql.Rows) error {
		var title, item string
		var ns int
		if err := rows.Scan(&title, &ns, &item); err != nil {
			return err
		}
		if e := pl.Get(wiki.NewTitle(title, ns)); e != nil {
			e.WikidataItem = item
		}
		return nil
	})
}

// convertNamespaces replaces pages by their talk pages or vice versa.
func (r *Run) convertNamespaces(_ context.Context, pl *PageList) error {
	var talk int
	switch r.params.GetDefault("namespace_conversion", "keep") {
	case "topic":
		talk = 0
	case "talk":
		talk = 1
	default:
		return nil
	}
	converted := NewPageList(pl.Wiki)
	for t := range pl.entries {
		converted.AddTitle(wiki.Title{NS: t.NS - t.NS%2 + talk, Name: t.Name})
	}
	pl.Replace(converted)
	return nil
}

// processSubpages adds subpages and filters on being a subpage.
func (r *Run) processSubpages(ctx context.Context, pl *PageList) error {
	if r.params.Has("add_subpages") {
		var queries []*replica.Query
		for t := range pl.entries {
			queries = append(queries, replica.New(
				"SELECT page_title,page_namespace FROM page WHERE page_namespace=? AND page_title LIKE ?",
				t.NS, escapeLike(t.Underscored())+"/%"))
		}
		var found []wiki.Title
		err := r.queryAll(ctx, pl.Wiki, replica.Core, queries, func(rows *sql.Rows) error {
			var title string
			var ns int
			if err := rows.Scan(&title, &ns); err != nil {
				return err
			}
			found = append(found, wiki.NewTitle(title, ns))
			return nil
		})
		if err != nil {
			return err
		}
		for _, t := range found {
			pl.AddTitle(t)
		}
	}
	switch r.params.GetDefault("subpage_filter", "either") {
	case "subpages":
		pl.Retain(func(e *Entry) bool { return strings.Contains(e.Title.Name, "/") })
	case "no_subpages":
		pl.Retain(func(e *Entry) bool { return !strings.Contains(e.Title.Name, "/") })
	}
	return nil
}

// escapeLike escapes the wildcards of a LIKE pattern.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// filterRegexp keeps pages whose title (Wikidata: label) matches rxp_filter
// as a whole.
func (r *Run) filterRegexp(_ context.Context, pl *PageList) error {
	if !r.params.Has("rxp_filter") {
		return nil
	}
	re, err := regexp.Compile("^(?:" + r.params.Get("rxp_filter") + ")$")
	if err != nil {
		return invalidf("invalid regular expression in rxp_filter: %v", err)
	}
	isWikidata := pl.IsWikidata()
	pl.Retain(func(e *Entry) bool {
		if isWikidata {
			return e.WikidataLabel != "" && re.MatchString(e.WikidataLabel)
		}
		return re.MatchString(e.Title.Name)
	})
	return nil
}

// maxSearchFilterPages bounds the per-page searches of search_filter.
const maxSearchFilterPages = 10000

// filterSearch keeps pages matching the full-text search search_filter.
func (r *Run) filterSearch(ctx context.Context, pl *PageList) error {
	search := r.params.Get("search_filter")
	if search == "" {
		return nil
	}
	if pl.Len() > maxSearchFilterPages {
		return invalidf("too many pages (%d) for the search filter, maximum is %d", pl.Len(), maxSearchFilterPages)
	}
	ids := pl.pageIDs()
	matches := make([]bool, len(ids))
	err := parallel(ctx, len(ids), maxParallelQueries, func(ctx context.Context, i int) error {
		hits, err := r.env.Wiki.Search(ctx, pl.Wiki, "pageid:"+strconv.FormatInt(ids[i], 10)+" "+search, "*", 1)
		matches[i] = len(hits) > 0
		return err
	})
	if err != nil {
		return upstream(fmt.Errorf("search filter: %w", err))
	}
	keep := map[int64]bool{}
	for i, id := range ids {
		if matches[i] {
			keep[id] = true
		}
	}
	pl.Retain(func(e *Entry) bool { return keep[e.PageID] })
	return nil
}

// processRedlinks replaces the result by the missing pages it links to, with
// the number of result pages linking to each.
func (r *Run) processRedlinks(ctx context.Context, pl *PageList) error {
	if !r.params.Has("show_redlinks") || pl.IsWikidata() || pl.Len() == 0 {
		return nil
	}
	var queries []*replica.Query
	for _, b := range pl.batches(pageBatchSize / 20) {
		q := replica.New("SELECT DISTINCT p.page_id,lt0.lt_namespace,lt0.lt_title FROM page p,pagelinks pl0,linktarget lt0 " +
			"WHERE pl0.pl_from=p.page_id AND pl0.pl_target_id=lt0.lt_id AND ").Append(b.whereP())
		if r.params.Has("article_redlinks_only") {
			q.Add(" AND lt0.lt_namespace=0")
		} else {
			q.Add(" AND lt0.lt_namespace>=0")
		}
		if r.params.Has("remove_template_redlinks") {
			q.Add(" AND NOT EXISTS (SELECT 1 FROM pagelinks pl1,linktarget lt1 WHERE pl1.pl_target_id=lt1.lt_id " +
				"AND pl1.pl_from_namespace=10 AND lt1.lt_namespace=lt0.lt_namespace AND lt1.lt_title=lt0.lt_title)")
		}
		q.Add(" AND NOT EXISTS (SELECT 1 FROM page p1 WHERE p1.page_namespace=lt0.lt_namespace AND p1.page_title=lt0.lt_title)")
		queries = append(queries, q)
	}
	cluster, err := replica.ClusterFor(pl.Wiki, "page", "pagelinks", "linktarget")
	if err != nil {
		return err
	}
	counts := map[wiki.Title]int{}
	err = r.queryAll(ctx, pl.Wiki, cluster, queries, func(rows *sql.Rows) error {
		var id int64
		var ns int
		var title string
		if err := rows.Scan(&id, &ns, &title); err != nil {
			return err
		}
		counts[wiki.NewTitle(title, ns)]++
		return nil
	})
	if err != nil {
		return err
	}
	minCount, ok := r.params.Int("min_redlink_count")
	if !ok {
		minCount = 1
	}
	redlinks := NewPageList(pl.Wiki)
	for t, n := range counts {
		if n >= minCount {
			redlinks.AddTitle(t).RedlinkCount = &n
		}
	}
	pl.Replace(redlinks)
	return nil
}
