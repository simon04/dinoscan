package petscan

import (
	"context"
	"database/sql"
	"strings"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// Filters on the Wikidata items of the result. Each converts the result to
// Wikidata, keeps the matching items and, where noted, converts back.

// filterWikidata applies the statement, identifier and sitelink counts and
// the "uses items/properties" filter (wikidata_prop_item_use).
func (r *Run) filterWikidata(ctx context.Context, pl *PageList) error {
	p := r.params
	conditions := new(replica.Query)
	for _, c := range []struct {
		key    string
		clause replica.Fragment
	}{
		{"min_statements", " AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=page_id AND pp_propname='wb-claims' AND pp_sortkey>=?)"},
		{"max_statements", " AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=page_id AND pp_propname='wb-claims' AND pp_sortkey<=?)"},
		{"min_identifiers", " AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=page_id AND pp_propname='wb-identifiers' AND pp_sortkey>=?)"},
		{"max_identifiers", " AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=page_id AND pp_propname='wb-identifiers' AND pp_sortkey<=?)"},
	} {
		if n, ok := p.Int(c.key); ok {
			conditions.Add(c.clause, n)
		}
	}
	if p.Has("wpiu_no_statements") {
		conditions.Add(" AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=page_id AND pp_propname='wb-claims' AND pp_sortkey=0)")
	}
	if p.Has("wpiu_no_sitelinks") {
		conditions.Add(" AND EXISTS (SELECT 1 FROM page_props WHERE pp_page=page_id AND pp_propname='wb-sitelinks' AND pp_sortkey=0)")
	}

	// Items linking to (i.e. using) the given items and properties.
	var uses []*replica.Query
	for _, id := range strings.Split(p.Get("wikidata_prop_item_use"), ",") {
		t, ok := entryForEntity(strings.TrimSpace(id))
		if !ok || t.NS == 146 {
			continue
		}
		uses = append(uses, replica.New("(SELECT 1 FROM pagelinks,linktarget WHERE pl_from=page_id AND pl_target_id=lt_id AND lt_namespace=? AND lt_title=?)", t.NS, t.Name))
	}
	if len(uses) > 0 {
		switch p.GetDefault("wpiu", "any") {
		case "all":
			for _, u := range uses {
				conditions.Add(" AND EXISTS ").Append(u)
			}
		case "any":
			conditions.Add(" AND (0=1")
			for _, u := range uses {
				conditions.Add(" OR EXISTS ").Append(u)
			}
			conditions.Add(")")
		case "none":
			for _, u := range uses {
				conditions.Add(" AND NOT EXISTS ").Append(u)
			}
		}
	}
	if conditions.Empty() || pl.Len() == 0 {
		return nil
	}

	return r.onWikidata(ctx, pl, func() error {
		var queries []*replica.Query
		for _, b := range pl.batches(pageBatchSize) {
			queries = append(queries, replica.New("SELECT page_title,page_namespace FROM page WHERE ").
				Append(b.where()).Append(conditions))
		}
		return r.replaceFromQueries(ctx, pl, wikidataWiki, replica.Core, queries)
	})
}

// onWikidata converts pl to Wikidata, calls fn and converts back.
func (r *Run) onWikidata(ctx context.Context, pl *PageList, fn func() error) error {
	original := pl.Wiki
	if err := r.convertToWiki(ctx, pl, wikidataWiki); err != nil {
		return err
	}
	if pl.Len() > 0 {
		if err := fn(); err != nil {
			return err
		}
	}
	return r.convertToWiki(ctx, pl, original)
}

// replaceFromQueries replaces the pages by those the queries return, as
// (page_title, page_namespace) rows.
func (r *Run) replaceFromQueries(ctx context.Context, pl *PageList, w string, cluster replica.Cluster, queries []*replica.Query) error {
	found := NewPageList(pl.Wiki)
	err := r.queryAll(ctx, w, cluster, queries, func(rows *sql.Rows) error {
		var title string
		var ns int
		if err := rows.Scan(&title, &ns); err != nil {
			return err
		}
		found.AddTitle(wiki.NewTitle(title, ns))
		return nil
	})
	if err != nil {
		return err
	}
	pl.Replace(found)
	return nil
}

// filterSitelinks keeps items with sitelinks to all of sitelinks_yes, any of
// sitelinks_any and none of sitelinks_no, and the min/max sitelink count.
func (r *Run) filterSitelinks(ctx context.Context, pl *PageList) error {
	p := r.params
	yes := p.List("sitelinks_yes", "\n")
	anyOf := p.List("sitelinks_any", "\n")
	none := p.List("sitelinks_no", "\n")
	minCount, hasMin := p.Int("min_sitelink_count")
	maxCount, hasMax := p.Int("max_sitelink_count")
	withCounts := p.Has("min_sitelink_count") || p.Has("max_sitelink_count")
	if len(yes) == 0 && len(anyOf) == 0 && len(none) == 0 && !withCounts {
		return nil
	}

	return r.onWikidata(ctx, pl, func() error {
		var queries []*replica.Query
		for _, b := range pl.batchesInNamespace(pageBatchSize, 0) {
			q := new(replica.Query)
			if withCounts {
				q.Add("SELECT page_title,page_namespace,(SELECT count(*) FROM wb_items_per_site WHERE ips_item_id=SUBSTRING(page_title,2)) AS sitelink_count")
			} else {
				q.Add("SELECT DISTINCT page_title,page_namespace")
			}
			q.Add(" FROM page WHERE ").Append(b.where())
			for _, site := range yes {
				q.Add(" AND EXISTS (SELECT 1 FROM wb_items_per_site WHERE ips_item_id=SUBSTRING(page_title,2) AND ips_site_id=?)", site)
			}
			if len(anyOf) > 0 {
				q.Add(" AND EXISTS (SELECT 1 FROM wb_items_per_site WHERE ips_item_id=SUBSTRING(page_title,2) AND ips_site_id").
					In(replica.Strings(anyOf)).Add(")")
			}
			for _, site := range none {
				q.Add(" AND NOT EXISTS (SELECT 1 FROM wb_items_per_site WHERE ips_item_id=SUBSTRING(page_title,2) AND ips_site_id=?)", site)
			}
			addSitelinkCountHaving(q, withCounts, minCount, hasMin, maxCount, hasMax)
			queries = append(queries, q)
		}
		found := NewPageList(wikidataWiki)
		err := r.queryAll(ctx, wikidataWiki, replica.Core, queries, func(rows *sql.Rows) error {
			var title string
			var ns int
			var count int
			dest := []any{&title, &ns}
			if withCounts {
				dest = append(dest, &count)
			}
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			e := found.AddTitle(wiki.NewTitle(title, ns))
			if withCounts {
				e.SitelinkCount = &count
			}
			return nil
		})
		if err != nil {
			return err
		}
		pl.Replace(found)
		return nil
	})
}

// filterLabels keeps the items (and properties) with labels, aliases or
// descriptions matching labels_yes (all), labels_any (any) and labels_no
// (none), each a LIKE pattern, in the languages of langs_labels_*.
func (r *Run) filterLabels(ctx context.Context, pl *PageList) error {
	p := r.params
	yes := p.List("labels_yes", "\n")
	anyOf := p.List("labels_any", "\n")
	none := p.List("labels_no", "\n")
	if len(yes)+len(anyOf)+len(none) == 0 {
		return nil
	}
	if err := r.convertToWiki(ctx, pl, wikidataWiki); err != nil {
		return err
	}
	if pl.Len() == 0 {
		return nil
	}

	var queries []*replica.Query
	for _, et := range []entityType{entityItem, entityProperty} {
		ns, prefix := 0, "Q"
		if et == entityProperty {
			ns, prefix = 120, "P"
		}
		for _, ids := range chunks(itemIDs(pl, ns, prefix), pageBatchSize) {
			var q *replica.Query
			if et == entityItem {
				q = replica.New("SELECT DISTINCT CONCAT('Q',t1.wbit_item_id),0 FROM wbt_item_terms t1 WHERE t1.wbit_item_id")
			} else {
				q = replica.New("SELECT DISTINCT CONCAT('P',t1.wbpt_property_id),120 FROM wbt_property_terms t1 WHERE t1.wbpt_property_id")
			}
			q.In(replica.Ints(ids))
			for _, pattern := range yes {
				q.Add(" AND EXISTS ").Append(r.labelSubquery(et, "yes", pattern))
			}
			if len(anyOf) > 0 {
				q.Add(" AND (0=1")
				for _, pattern := range anyOf {
					q.Add(" OR EXISTS ").Append(r.labelSubquery(et, "any", pattern))
				}
				q.Add(")")
			}
			for _, pattern := range none {
				q.Add(" AND NOT EXISTS ").Append(r.labelSubquery(et, "no", pattern))
			}
			queries = append(queries, q)
		}
	}
	return r.replaceFromQueries(ctx, pl, wikidataWiki, replica.TermStore, queries)
}

// labelSubquery matches terms of the entity t1 like pattern, of the term
// types and languages chosen for group ("yes", "any" or "no").
func (r *Run) labelSubquery(et entityType, group, pattern string) *replica.Query {
	q := new(replica.Query)
	if et == entityItem {
		q.Add("(SELECT 1 FROM wbt_item_terms t2,wbt_term_in_lang,wbt_text_in_lang,wbt_text " +
			"WHERE t2.wbit_item_id=t1.wbit_item_id AND wbtl_id=t2.wbit_term_in_lang_id")
	} else {
		q.Add("(SELECT 1 FROM wbt_property_terms t2,wbt_term_in_lang,wbt_text_in_lang,wbt_text " +
			"WHERE t2.wbpt_property_id=t1.wbpt_property_id AND wbtl_id=t2.wbpt_term_in_lang_id")
	}
	q.Add(" AND wbtl_text_in_lang_id=wbxl_id AND wbxl_text_id=wbx_id")

	// Term types: 1 label, 2 description, 3 alias.
	var types []any
	for _, t := range []struct {
		suffix string
		id     int
	}{{"l", 1}, {"a", 3}, {"d", 2}} {
		if r.params.Has("cb_labels_" + group + "_" + t.suffix) {
			types = append(types, t.id)
		}
	}
	if len(types) > 0 {
		q.Add(" AND wbtl_type_id").In(types)
	}
	if languages := r.params.List("langs_labels_"+group, ","); len(languages) > 0 {
		q.Add(" AND wbxl_language").In(replica.Strings(languages))
	}
	if pattern != "%" {
		// Spaces became underscores, which LIKE matches as any character.
		q.Add(" AND wbx_text LIKE ?", pattern)
	}
	return q.Add(")")
}
