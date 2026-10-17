package petscan

import (
	"sort"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// wikidataWiki is the database name of Wikidata.
const wikidataWiki = "wikidatawiki"

// PageList is a set of pages on one wiki.
type PageList struct {
	Wiki    string
	entries map[wiki.Title]*Entry
	// HasSitelinkCounts is set when the source already provided the
	// sitelink counts.
	HasSitelinkCounts bool
}

// NewPageList returns an empty list for wiki.
func NewPageList(w string) *PageList {
	return &PageList{Wiki: w, entries: map[wiki.Title]*Entry{}}
}

// IsWikidata reports whether the list holds Wikidata entities.
func (pl *PageList) IsWikidata() bool {
	return pl.Wiki == wikidataWiki
}

// Len returns the number of pages.
func (pl *PageList) Len() int {
	return len(pl.entries)
}

// Add adds or replaces the entry for e.Title.
func (pl *PageList) Add(e *Entry) {
	pl.entries[e.Title] = e
}

// AddTitle adds a page with no data but its title, unless it is present.
func (pl *PageList) AddTitle(t wiki.Title) *Entry {
	if e, ok := pl.entries[t]; ok {
		return e
	}
	e := &Entry{Title: t}
	pl.entries[t] = e
	return e
}

// Get returns the entry for t, or nil.
func (pl *PageList) Get(t wiki.Title) *Entry {
	return pl.entries[t]
}

// Clear removes all pages.
func (pl *PageList) Clear() {
	pl.entries = map[wiki.Title]*Entry{}
}

// Replace takes the wiki and pages of other.
func (pl *PageList) Replace(other *PageList) {
	pl.Wiki = other.Wiki
	pl.entries = other.entries
	pl.HasSitelinkCounts = other.HasSitelinkCounts
}

// Retain keeps the pages for which keep returns true.
func (pl *PageList) Retain(keep func(*Entry) bool) {
	for t, e := range pl.entries {
		if !keep(e) {
			delete(pl.entries, t)
		}
	}
}

// Entries returns the entries in no particular order.
func (pl *PageList) Entries() []*Entry {
	ret := make([]*Entry, 0, len(pl.entries))
	for _, e := range pl.entries {
		ret = append(ret, e)
	}
	return ret
}

// Union adds the pages of other, which must be on the same wiki.
func (pl *PageList) Union(other *PageList) {
	for t, e := range other.entries {
		if _, ok := pl.entries[t]; !ok {
			pl.entries[t] = e
		}
	}
}

// Intersect keeps the pages also in other.
func (pl *PageList) Intersect(other *PageList) {
	pl.Retain(func(e *Entry) bool { return other.entries[e.Title] != nil })
}

// Subtract removes the pages in other.
func (pl *PageList) Subtract(other *PageList) {
	pl.Retain(func(e *Entry) bool { return other.entries[e.Title] == nil })
}

// checkLimit fails if the list holds more than limit pages.
func (pl *PageList) checkLimit(limit int) error {
	if pl.Len() > limit {
		return tooManyResults("this query yields", limit)
	}
	return nil
}

// tooManyResults is the error for a query that would load more than limit
// pages; what completes "Too many results: …".
func tooManyResults(what string, limit int) error {
	return invalidf("Too many results: %s more than %d pages, the most DinoScan handles in one query. "+
		"Please narrow it down, e.g. with less category depth, more filters, or a more specific SPARQL query", what, limit)
}

// titleBatch is a chunk of titles of one namespace, for an IN list.
type titleBatch struct {
	NS     int
	Titles []string // with underscores
}

// where returns "(page_namespace=? AND page_title IN (…))".
func (b titleBatch) where() *replica.Query {
	return replica.New("(page_namespace=? AND page_title", b.NS).In(replica.Strings(b.Titles)).Add(")")
}

// whereP is where for the table alias p.
func (b titleBatch) whereP() *replica.Query {
	return replica.New("(p.page_namespace=? AND p.page_title", b.NS).In(replica.Strings(b.Titles)).Add(")")
}

// batches groups the titles by namespace, in chunks of at most size, in a
// deterministic order.
func (pl *PageList) batches(size int) []titleBatch {
	byNS := map[int][]string{}
	for t := range pl.entries {
		byNS[t.NS] = append(byNS[t.NS], t.Underscored())
	}
	namespaces := make([]int, 0, len(byNS))
	for ns := range byNS {
		namespaces = append(namespaces, ns)
	}
	sort.Ints(namespaces)
	var ret []titleBatch
	for _, ns := range namespaces {
		titles := byNS[ns]
		sort.Strings(titles)
		for _, chunk := range chunks(titles, size) {
			ret = append(ret, titleBatch{NS: ns, Titles: chunk})
		}
	}
	return ret
}

// batchesInNamespace is batches for the pages in namespace ns only.
func (pl *PageList) batchesInNamespace(size, ns int) []titleBatch {
	var ret []titleBatch
	for _, b := range pl.batches(size) {
		if b.NS == ns {
			ret = append(ret, b)
		}
	}
	return ret
}

// pageIDs returns the known page IDs.
func (pl *PageList) pageIDs() []int64 {
	var ids []int64
	for _, e := range pl.entries {
		if e.PageID != 0 {
			ids = append(ids, e.PageID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// chunks splits s into parts of at most size elements.
func chunks[T any](s []T, size int) [][]T {
	var ret [][]T
	for len(s) > size {
		ret = append(ret, s[:size])
		s = s[size:]
	}
	if len(s) > 0 {
		ret = append(ret, s)
	}
	return ret
}
