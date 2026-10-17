package petscan

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// convertToWiki replaces the pages of pl by the corresponding pages on
// target, via their Wikidata items. Pages without a counterpart are dropped.
func (r *Run) convertToWiki(ctx context.Context, pl *PageList, target string) error {
	if pl.Wiki == "" || pl.Wiki == target {
		return nil
	}
	if !pl.IsWikidata() {
		if err := r.convertToWikidata(ctx, pl); err != nil {
			return err
		}
	}
	if target != wikidataWiki {
		return r.convertFromWikidata(ctx, pl, target)
	}
	return nil
}

// convertToWikidata replaces the pages by their Wikidata items.
func (r *Run) convertToWikidata(ctx context.Context, pl *PageList) error {
	var queries []*replica.Query
	for _, b := range pl.batches(pageBatchSize) {
		queries = append(queries, replica.New(
			"SELECT pp_value FROM page_props,page WHERE page_id=pp_page AND pp_propname='wikibase_item' AND ",
		).Append(b.where()))
	}
	items := NewPageList(wikidataWiki)
	err := r.queryAll(ctx, pl.Wiki, replica.Core, queries, func(rows *sql.Rows) error {
		var item string
		if err := rows.Scan(&item); err != nil {
			return err
		}
		items.AddTitle(wiki.NewTitle(item, 0))
		return nil
	})
	if err != nil {
		return err
	}
	pl.Replace(items)
	return nil
}

// convertFromWikidata replaces Wikidata items by their sitelinks to target.
func (r *Run) convertFromWikidata(ctx context.Context, pl *PageList, target string) error {
	ids := itemIDs(pl, 0, "Q")
	pages := NewPageList(target)
	if len(ids) == 0 {
		pl.Replace(pages)
		return nil
	}
	info, err := r.siteInfo(ctx, target)
	if err != nil {
		return err
	}
	var queries []*replica.Query
	for _, chunk := range chunks(ids, 2*pageBatchSize) {
		queries = append(queries, replica.New(
			"SELECT ips_site_page FROM wb_items_per_site WHERE ips_site_id=? AND ips_item_id", target,
		).In(replica.Ints(chunk)))
	}
	err = r.queryAll(ctx, wikidataWiki, replica.Core, queries, func(rows *sql.Rows) error {
		var page string
		if err := rows.Scan(&page); err != nil {
			return err
		}
		pages.AddTitle(info.ParseTitle(page))
		return nil
	})
	if err != nil {
		return err
	}
	pl.Replace(pages)
	return nil
}

// itemIDs returns the numeric IDs of the entities in namespace ns whose
// titles have prefix, e.g. 42 for Q42.
func itemIDs(pl *PageList, ns int, prefix string) []int64 {
	var ids []int64
	for t := range pl.entries {
		if t.NS != ns {
			continue
		}
		if digits, ok := strings.CutPrefix(t.Name, prefix); ok {
			if id, err := strconv.ParseInt(digits, 10, 64); err == nil {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// loadMissingMetadata adds page ID, size and last change where unknown, and
// Wikidata labels and descriptions in language for Wikidata results.
func (r *Run) loadMissingMetadata(ctx context.Context, pl *PageList, language string) error {
	if err := r.loadPageMetadata(ctx, pl); err != nil {
		return err
	}
	if !pl.IsWikidata() || language == "" {
		return nil
	}
	if err := r.loadWikidataTerms(ctx, pl, entityItem, language); err != nil {
		return err
	}
	return r.loadWikidataTerms(ctx, pl, entityProperty, language)
}

func (r *Run) loadPageMetadata(ctx context.Context, pl *PageList) error {
	missing := false
	for _, e := range pl.entries {
		if e.PageID == 0 || e.Touched == "" {
			missing = true
			break
		}
	}
	if !missing {
		return nil
	}
	var queries []*replica.Query
	for _, b := range pl.batches(pageBatchSize) {
		queries = append(queries, replica.New(
			"SELECT page_title,page_namespace,page_id,page_len,"+
				"(SELECT rev_timestamp FROM revision WHERE rev_id=page_latest LIMIT 1) "+
				"FROM page WHERE ",
		).Append(b.where()))
	}
	return r.queryAll(ctx, pl.Wiki, replica.Core, queries, func(rows *sql.Rows) error {
		var (
			title    string
			ns       int
			id, size int64
			touched  sql.NullString
		)
		if err := rows.Scan(&title, &ns, &id, &size, &touched); err != nil {
			return err
		}
		if e := pl.Get(wiki.NewTitle(title, ns)); e != nil {
			e.PageID, e.Len, e.Touched = id, size, touched.String
		}
		return nil
	})
}

// entityType distinguishes Wikidata items from properties, whose terms are
// in different tables.
type entityType int

const (
	entityItem entityType = iota
	entityProperty
)

// loadWikidataTerms adds labels and descriptions in language from the term
// store.
func (r *Run) loadWikidataTerms(ctx context.Context, pl *PageList, et entityType, language string) error {
	ns, prefix := 0, "Q"
	if et == entityProperty {
		ns, prefix = 120, "P"
	}
	ids := itemIDs(pl, ns, prefix)
	var queries []*replica.Query
	for _, chunk := range chunks(ids, pageBatchSize) {
		var q *replica.Query
		if et == entityItem {
			q = replica.New("SELECT wbit_item_id,wbtl_type_id,wbx_text FROM wbt_item_terms " +
				"INNER JOIN wbt_term_in_lang ON wbit_term_in_lang_id=wbtl_id ")
		} else {
			q = replica.New("SELECT wbpt_property_id,wbtl_type_id,wbx_text FROM wbt_property_terms " +
				"INNER JOIN wbt_term_in_lang ON wbpt_term_in_lang_id=wbtl_id ")
		}
		q.Add("INNER JOIN wbt_text_in_lang ON wbtl_text_in_lang_id=wbxl_id "+
			"INNER JOIN wbt_text ON wbxl_text_id=wbx_id "+
			"WHERE wbtl_type_id IN (1,2) AND wbxl_language=? AND ", language)
		if et == entityItem {
			q.Add("wbit_item_id")
		} else {
			q.Add("wbpt_property_id")
		}
		queries = append(queries, q.In(replica.Ints(chunk)))
	}
	return r.queryAll(ctx, wikidataWiki, replica.TermStore, queries, func(rows *sql.Rows) error {
		var (
			id       int64
			termType int
			text     string
		)
		if err := rows.Scan(&id, &termType, &text); err != nil {
			return err
		}
		e := pl.Get(wiki.Title{NS: ns, Name: prefix + strconv.FormatInt(id, 10)})
		if e == nil {
			return nil
		}
		switch termType {
		case 1:
			e.WikidataLabel = text
		case 2:
			e.WikidataDescription = text
		}
		return nil
	})
}
