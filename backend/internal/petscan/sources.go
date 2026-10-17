package petscan

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// The data sources besides "categories", see source_database.go.

// entryForEntity returns the Wikidata page of an entity ID like Q42, P31 or
// L1, or false.
func entryForEntity(id string) (wiki.Title, bool) {
	if len(id) < 2 {
		return wiki.Title{}, false
	}
	switch id[0] {
	case 'Q':
		return wiki.Title{NS: 0, Name: id}, true
	case 'P':
		return wiki.Title{NS: 120, Name: id}, true
	case 'L':
		return wiki.Title{NS: 146, Name: id}, true
	}
	return wiki.Title{}, false
}

// --- SPARQL ---

const (
	sparqlTimeout = 10 * time.Minute
	// sparqlBytesPerRow is what a response may average per allowed row;
	// it bounds what is buffered before the rows can be counted.
	sparqlBytesPerRow = 300
	qleverPrefixes    = "PREFIX wikibase: <http://wikiba.se/ontology#>\n" +
		"PREFIX wd: <http://www.wikidata.org/entity/>\n" +
		"PREFIX wdt: <http://www.wikidata.org/prop/direct/>\n"
)

func canRunSPARQL(p Params) bool { return p.Has("sparql") }

func runSPARQL(ctx context.Context, r *Run) (*PageList, error) {
	endpoint, query := "https://query.wikidata.org/sparql", r.params.Get("sparql")
	if r.params.Get("sparql_server") == "qlever_wd" {
		endpoint, query = "https://qlever.cs.uni-freiburg.de/api/wikidata", qleverPrefixes+query
	}
	ctx, cancel := context.WithTimeout(ctx, sparqlTimeout)
	defer cancel()
	form := url.Values{"query": {query}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", wiki.UserAgent)
	client := &http.Client{Timeout: sparqlTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, upstream(fmt.Errorf("SPARQL: %w", err))
	}
	defer resp.Body.Close()

	limit := r.env.MaxResults
	maxBytes := int64(limit) * sparqlBytesPerRow
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("SPARQL: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, tooManyResults(fmt.Sprintf("the SPARQL result exceeds %d MB, so it likely has", maxBytes/1_000_000), limit)
	}
	if resp.StatusCode/100 != 2 {
		if resp.StatusCode/100 == 4 {
			return nil, invalidf("SPARQL query failed: %s", snippet(body))
		}
		return nil, upstream(fmt.Errorf("SPARQL endpoint returned HTTP %d: %s", resp.StatusCode, snippet(body)))
	}
	return parseSPARQL(body, limit)
}

// parseSPARQL reads the entities of the first variable of a SPARQL JSON
// result.
func parseSPARQL(body []byte, limit int) (*PageList, error) {
	// The endpoints occasionally emit stray control characters.
	body = bytes.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return ' '
		}
		return r
	}, body)
	var result struct {
		Head struct {
			Vars []string `json:"vars"`
		} `json:"head"`
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, upstream(fmt.Errorf("SPARQL endpoint did not return valid JSON (%v); this is usually a transient "+
			"endpoint error (timeout, rate limit, or maintenance), please retry. Response began: %s", err, snippet(body)))
	}
	if len(result.Results.Bindings) > limit {
		return nil, tooManyResults("the SPARQL result has", limit)
	}
	if len(result.Head.Vars) == 0 {
		return nil, invalidf("no variables found in SPARQL result")
	}
	first := result.Head.Vars[0]
	pl := NewPageList(wikidataWiki)
	for _, row := range result.Results.Bindings {
		value, ok := row[first]
		if !ok {
			continue
		}
		_, id, found := strings.Cut(value.Value, "/entity/")
		if !found {
			continue
		}
		if t, ok := entryForEntity(id); ok {
			pl.AddTitle(t)
		}
	}
	return pl, nil
}

func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	return strconv.Quote(s)
}

// --- Manual list ---

func canRunManual(p Params) bool { return p.Has("manual_list") && p.Has("manual_list_wiki") }

func runManual(ctx context.Context, r *Run) (*PageList, error) {
	w := r.params.Get("manual_list_wiki")
	site, err := r.siteInfo(ctx, w)
	if err != nil {
		return nil, err
	}
	pl := NewPageList(w)
	for _, line := range strings.Split(r.params.Get("manual_list"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			pl.AddTitle(site.ParseTitle(line))
		}
	}
	return pl, nil
}

// --- PagePile ---

const pagePileAPI = "https://pagepile.toolforge.org/api.php"

func canRunPagePile(p Params) bool { return p.Has("pagepile") }

func runPagePile(ctx context.Context, r *Run) (*PageList, error) {
	id := r.params.Get("pagepile")
	u := pagePileAPI + "?" + url.Values{"id": {id}, "action": {"get_data"}, "format": {"json"}, "doit": {"1"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var pile struct {
		Wiki  string   `json:"wiki"`
		Pages []string `json:"pages"`
	}
	if err := r.env.Wiki.Do(req, &pile); err != nil {
		return nil, upstream(fmt.Errorf("PagePile %s: %w", id, err))
	}
	if pile.Wiki == "" {
		return nil, invalidf("PagePile %s does not specify a wiki", id)
	}
	if len(pile.Pages) > r.env.MaxResults {
		return nil, tooManyResults("PagePile "+id+" holds", r.env.MaxResults)
	}
	site, err := r.siteInfo(ctx, pile.Wiki)
	if err != nil {
		return nil, err
	}
	pl := NewPageList(pile.Wiki)
	for _, page := range pile.Pages {
		pl.AddTitle(site.ParseTitle(page))
	}
	return pl, nil
}

// --- Search ---

func canRunSearch(p Params) bool {
	return strings.TrimSpace(p.Get("search_query")) != "" &&
		strings.TrimSpace(p.Get("search_wiki")) != "" &&
		p.Has("search_max_results")
}

func runSearch(ctx context.Context, r *Run) (*PageList, error) {
	w := r.params.Get("search_wiki")
	maxResults, ok := r.params.Int("search_max_results")
	if !ok {
		return nil, invalidf("invalid search_max_results %q", r.params.Get("search_max_results"))
	}
	namespaces := "*"
	if len(r.params.Namespaces) > 0 {
		parts := make([]string, len(r.params.Namespaces))
		for i, ns := range r.params.Namespaces {
			parts[i] = strconv.Itoa(ns)
		}
		namespaces = strings.Join(parts, "|")
	}
	hits, err := r.env.Wiki.Search(ctx, w, r.params.Get("search_query"), namespaces, maxResults)
	if err != nil {
		return nil, upstream(err)
	}
	pl := NewPageList(w)
	for _, hit := range hits {
		name := hit.Title
		if hit.NS != 0 {
			if _, rest, found := strings.Cut(name, ":"); found {
				name = rest
			}
		}
		pl.AddTitle(wiki.NewTitle(name, hit.NS)).PageID = hit.PageID
	}
	return pl, nil
}

// --- Wikidata items with sitelinks to given sites ---

func canRunWikidata(p Params) bool {
	return p.Has("wpiu_no_statements") && p.Has("wikidata_source_sites")
}

func runWikidata(ctx context.Context, r *Run) (*PageList, error) {
	var sites []string
	for _, s := range strings.Split(r.params.Get("wikidata_source_sites"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			sites = append(sites, s)
		}
	}
	if len(sites) == 0 {
		return nil, invalidf("no Wikidata source sites given")
	}
	// Items without statements, with a sitelink to any of the sites.
	q := replica.New("SELECT DISTINCT ips_item_id FROM wb_items_per_site,page_props,page WHERE ips_site_id").
		In(replica.Strings(sites)).
		Add(" AND page_namespace=0 AND page_title=CONCAT('Q',ips_item_id) "+
			"AND pp_page=page_id AND pp_propname='wb-claims' AND pp_sortkey=0 LIMIT ?", r.env.MaxResults+1)
	pl := NewPageList(wikidataWiki)
	err := r.query(ctx, wikidataWiki, replica.Core, q, func(rows *sql.Rows) error {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		pl.AddTitle(wiki.Title{NS: 0, Name: "Q" + strconv.FormatInt(id, 10)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pl, pl.checkLimit(r.env.MaxResults)
}

// --- Pages with language links to given wikis ---

func canRunSitelinks(p Params) bool { return p.Has("sitelinks_yes") || p.Has("sitelinks_any") }

func runSitelinks(ctx context.Context, r *Run) (*PageList, error) {
	yes := r.params.List("sitelinks_yes", "\n")
	anyOf := r.params.List("sitelinks_any", "\n")
	none := r.params.List("sitelinks_no", "\n")
	minCount, hasMin := r.params.Int("min_sitelink_count")
	maxCount, hasMax := r.params.Int("max_sitelink_count")
	withCounts := r.params.Has("min_sitelink_count") || r.params.Has("max_sitelink_count")

	mainWiki := append(append([]string{}, yes...), anyOf...)[0]
	if _, err := r.siteInfo(ctx, mainWiki); err != nil {
		return nil, err // also rejects unknown wikis before connecting
	}
	// The other wikis' language codes, e.g. "de" for dewiki.
	languages := func(sites []string) []string {
		var ret []string
		for _, s := range sites {
			if s != mainWiki {
				ret = append(ret, strings.TrimSuffix(s, "wiki"))
			}
		}
		return ret
	}

	q := new(replica.Query)
	if withCounts {
		q.Add("SELECT page_title,(SELECT count(*) FROM langlinks WHERE ll_from=page_id) AS sitelink_count")
	} else {
		q.Add("SELECT DISTINCT page_title,0")
	}
	q.Add(" FROM page WHERE page_namespace=0")
	for _, lang := range languages(yes) {
		q.Add(" AND page_id IN (SELECT ll_from FROM langlinks WHERE ll_lang=?)", lang)
	}
	if langs := languages(anyOf); len(langs) > 0 {
		q.Add(" AND page_id IN (SELECT ll_from FROM langlinks WHERE ll_lang").In(replica.Strings(langs)).Add(")")
	}
	if langs := languages(none); len(langs) > 0 {
		q.Add(" AND page_id NOT IN (SELECT ll_from FROM langlinks WHERE ll_lang").In(replica.Strings(langs)).Add(")")
	}
	addSitelinkCountHaving(q, withCounts, minCount, hasMin, maxCount, hasMax)
	q.Add(" LIMIT ?", r.env.MaxResults+1)

	cluster, err := replica.ClusterFor(mainWiki, "page", "langlinks")
	if err != nil {
		return nil, err
	}
	pl := NewPageList(mainWiki)
	pl.HasSitelinkCounts = withCounts
	err = r.query(ctx, mainWiki, cluster, q, func(rows *sql.Rows) error {
		var title string
		var count int
		if err := rows.Scan(&title, &count); err != nil {
			return err
		}
		e := pl.AddTitle(wiki.NewTitle(title, 0))
		if withCounts {
			e.SitelinkCount = &count
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pl, pl.checkLimit(r.env.MaxResults)
}

// addSitelinkCountHaving groups by page and filters on sitelink_count.
func addSitelinkCountHaving(q *replica.Query, withCounts bool, minCount int, hasMin bool, maxCount int, hasMax bool) {
	if !withCounts {
		return
	}
	q.Add(" GROUP BY page_title")
	switch {
	case hasMin && hasMax:
		q.Add(" HAVING sitelink_count>=? AND sitelink_count<=?", minCount, maxCount)
	case hasMin:
		q.Add(" HAVING sitelink_count>=?", minCount)
	case hasMax:
		q.Add(" HAVING sitelink_count<=?", maxCount)
	}
}
