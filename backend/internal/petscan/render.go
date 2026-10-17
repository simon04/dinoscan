package petscan

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// Response is a rendered result.
type Response struct {
	Status      int
	ContentType string
	Header      http.Header
	Body        []byte
}

// Formats are the supported values of the format parameter.
var Formats = []string{"json", "jsonl", "csv", "tsv", "wiki", "kml", "plain", "pagepile"}

// renderOptions are the output options of the query.
type renderOptions struct {
	showWikidataItem bool
	addImage         bool
	addCoordinates   bool
	addDefaultSort   bool
	addDisambig      bool
	addIncomingLinks bool
	addSitelinks     bool
	fileData         bool
	fileUsage        bool
	redlinks         bool
}

func (r *Run) renderOptions() renderOptions {
	p := r.params
	wdi := p.GetDefault("wikidata_item", "no")
	return renderOptions{
		showWikidataItem: wdi == "any" || wdi == "with",
		addImage:         p.Has("add_image") || p.Get("format") == "kml",
		addCoordinates:   p.Has("add_coordinates"),
		addDefaultSort:   p.Has("add_defaultsort"),
		addDisambig:      p.Has("add_disambiguation"),
		addIncomingLinks: p.Get("sortby") == "incoming_links",
		addSitelinks:     p.Get("sortby") == "sitelinks",
		fileData:         p.Has("ext_image_data"),
		fileUsage:        p.Has("file_usage_data") || p.Has("giu"),
		redlinks:         p.Has("show_redlinks"),
	}
}

// columns are the optional output columns, in PetScan's order.
func (o renderOptions) columns() []string {
	cols := []string{"number"}
	if o.addImage {
		cols = append(cols, "image")
	}
	cols = append(cols, "title")
	if o.redlinks {
		cols = append(cols, "redlink_count")
	} else {
		cols = append(cols, "page_id", "namespace", "size", "timestamp")
	}
	for _, c := range []struct {
		on  bool
		col string
	}{
		{o.showWikidataItem, "wikidata_item"},
		{o.addCoordinates, "coordinates"},
		{o.addDefaultSort, "defaultsort"},
		{o.addDisambig, "disambiguation"},
		{o.addIncomingLinks, "incoming_links"},
		{o.addSitelinks, "sitelinks"},
	} {
		if c.on {
			cols = append(cols, c.col)
		}
	}
	if o.fileData {
		cols = append(cols, fileDataKeys...)
	}
	if o.fileUsage {
		cols = append(cols, "fileusage")
	}
	return cols
}

// column is an output column: its key and its header label.
type column struct{ key, label string }

// withColumns appends the optional columns not yet in header, except
// "number".
func withColumns(header []column, o renderOptions) []column {
	for _, key := range o.columns() {
		if key == "number" {
			continue
		}
		present := false
		for _, c := range header {
			present = present || c.key == key
		}
		if !present {
			header = append(header, column{key, key})
		}
	}
	return header
}

// Render sorts, truncates and renders the result in the requested format.
func (r *Run) Render(ctx context.Context) (*Response, error) {
	if r.result == nil {
		return nil, fmt.Errorf("no result")
	}
	site, err := r.siteInfo(ctx, r.result.Wiki)
	if err != nil {
		return nil, upstream(err)
	}
	entries := r.sortedEntries()
	switch format := r.params.Format(); format {
	case "json":
		return r.renderJSON(entries, site)
	case "jsonl":
		return r.renderJSONL(entries, site)
	case "csv", "tsv":
		return r.renderTable(entries, site, format)
	case "wiki":
		return r.renderWiki(entries, site), nil
	case "kml":
		return r.renderKML(entries, site), nil
	case "plain":
		return r.renderPlain(entries, site), nil
	case "pagepile":
		return r.renderPagePile(ctx, entries)
	default:
		return nil, invalidf("unsupported format %q, use one of %s", format, strings.Join(Formats, ", "))
	}
}

// sortedEntries applies sortby, sortorder and output_limit.
func (r *Run) sortedEntries() []*Entry {
	sortBy := r.params.Get("sortby")
	descending := r.params.Get("sortorder") == "descending"
	if r.params.Has("show_redlinks") && (sortBy == "" || sortBy == "none") {
		sortBy, descending = "redlinks", true
	}
	entries := r.result.Entries()
	sortEntries(entries, sortBy, descending, r.result.IsWikidata())
	if limit, ok := r.params.Int("output_limit"); ok && limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}
	return entries
}

// queryURL links to the API with this query.
func (r *Run) queryURL(without ...string) string {
	return strings.TrimSuffix(r.env.PublicURL, "/") + "/api?" + r.params.Encode(without...)
}

// --- JSON ---

func (r *Run) renderJSON(entries []*Entry, site *wiki.SiteInfo) (*Response, error) {
	value := r.jsonValue(entries, site)
	var body []byte
	var err error
	contentType := "application/json"
	if r.params.Has("json-pretty") {
		body, err = json.MarshalIndent(value, "", "  ")
		contentType = "text/plain; charset=utf-8"
	} else {
		body, err = json.Marshal(value)
	}
	if err != nil {
		return nil, err
	}
	if callback := r.params.Get("callback"); callback != "" {
		if !isJSIdentifier(callback) {
			return nil, invalidf("invalid callback name")
		}
		body = append(append([]byte(callback+"("), body...), ')')
		contentType = "application/javascript"
	}
	return &Response{Status: http.StatusOK, ContentType: contentType, Body: body}, nil
}

func (r *Run) renderJSONL(entries []*Entry, site *wiki.SiteInfo) (*Response, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, item := range r.jsonItems(entries, site) {
		if err := enc.Encode(item); err != nil {
			return nil, err
		}
	}
	return &Response{Status: http.StatusOK, ContentType: "application/x-ndjson", Body: buf.Bytes()}, nil
}

// isJSIdentifier accepts JSONP callback names like "jQuery123_456" or
// "a.b", nothing that could inject script.
func isJSIdentifier(s string) bool {
	for _, c := range s {
		if !(c == '_' || c == '$' || c == '.' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return s != ""
}

func (r *Run) quickIntersection() bool {
	return r.params.GetDefault("output_compatability", "catscan") == "quick-intersection"
}

// jsonHeader returns the columns whose values go into "metadata".
func (r *Run) jsonHeader() []column {
	o := r.renderOptions()
	header := []column{{"title", ""}, {"page_id", ""}, {"namespace", ""}, {"size", ""}, {"timestamp", ""}}
	if o.showWikidataItem {
		header = append(header, column{"wikidata_item", ""})
	}
	return withColumns(header, o)
}

// jsonItems returns the JSON value of each entry.
func (r *Run) jsonItems(entries []*Entry, site *wiki.SiteInfo) []any {
	items := make([]any, 0, len(entries))
	if r.params.Has("sparse") && !r.params.Has("giu") {
		for _, e := range entries {
			if full, ok := site.FullUnderscored(e.Title); ok {
				items = append(items, full)
			}
		}
		return items
	}
	header := r.jsonHeader()
	fileData := r.renderOptions().fileData
	qi := r.quickIntersection()
	for _, e := range entries {
		metadata := entryMetadata(e, header)
		var o map[string]any
		if qi {
			o = map[string]any{
				"page_id":        e.PageID,
				"page_namespace": e.Title.NS,
				"page_title":     e.Title.Underscored(),
				"page_latest":    e.Touched,
				"page_len":       e.Len,
			}
			if e.File != nil && len(e.File.Usage) > 0 && (r.params.Has("giu") || r.params.Has("file_usage_data")) {
				usage := make([]any, len(e.File.Usage))
				for i, u := range e.File.Usage {
					usage[i] = map[string]any{"ns": u.Title.NS, "page": u.Title.Underscored(), "wiki": u.Wiki}
				}
				o["giu"] = usage
			}
		} else {
			o = map[string]any{
				"n":         "page",
				"title":     e.Title.Underscored(),
				"id":        e.PageID,
				"namespace": e.Title.NS,
				"len":       e.Len,
				"touched":   e.Touched,
				"nstext":    site.CanonicalName(e.Title.NS),
			}
			if fileData {
				if v, ok := metadata["fileusage"]; ok {
					o["gil"] = v
				}
				for _, k := range fileDataKeys {
					if v, ok := metadata[k]; ok {
						o[k] = v
					}
				}
			}
		}
		if e.WikidataItem != "" {
			o["q"] = e.WikidataItem
			metadata["wikidata"] = e.WikidataItem
		}
		o["metadata"] = metadata
		items = append(items, o)
	}
	return items
}

// entryMetadata returns the optional values of the header columns.
func entryMetadata(e *Entry, header []column) map[string]any {
	m := map[string]any{}
	for _, c := range header {
		switch c.key {
		case "image":
			if e.Image != "" {
				m[c.key] = e.Image
			}
		case "defaultsort":
			if e.DefaultSort != "" {
				m[c.key] = e.DefaultSort
			}
		case "disambiguation":
			m[c.key] = e.Disambiguation.json()
		case "incoming_links":
			if e.IncomingLinks != nil {
				m[c.key] = *e.IncomingLinks
			}
		case "sitelinks":
			if e.SitelinkCount != nil {
				m[c.key] = *e.SitelinkCount
			}
		case "redlink_count":
			if e.RedlinkCount != nil {
				m[c.key] = *e.RedlinkCount
			}
		case "coordinates":
			if e.Coordinates != nil {
				m[c.key] = formatCoordinates(e.Coordinates)
			}
		case "fileusage":
			if e.File != nil && len(e.File.Usage) > 0 {
				m[c.key] = e.File.usageString(true)
			}
		default:
			if v, ok := e.File.field(c.key); ok {
				m[c.key] = v
			}
		}
	}
	return m
}

func formatCoordinates(c *Coordinates) string {
	return strconv.FormatFloat(c.Lat, 'f', -1, 64) + "/" + strconv.FormatFloat(c.Lon, 'f', -1, 64)
}

func (r *Run) jsonValue(entries []*Entry, site *wiki.SiteInfo) any {
	items := r.jsonItems(entries, site)
	seconds := r.queryTime.Seconds()
	if r.quickIntersection() {
		namespaces := map[string]string{}
		for _, id := range site.SortedNamespaceIDs() {
			name, _ := site.LocalName(id)
			namespaces[strconv.Itoa(id)] = name
		}
		mainWiki, _ := r.mainWiki()
		return map[string]any{
			"namespaces": namespaces,
			"wiki":       mainWiki,
			"status":     "OK",
			"start":      0,
			"max":        len(entries) + 1,
			"query":      r.queryURL(),
			"pagecount":  len(entries),
			"pages":      items,
			"querytime":  seconds,
		}
	}
	return map[string]any{
		"n": "result",
		"a": map[string]any{"query": r.queryURL(), "querytime_sec": seconds},
		"*": []any{map[string]any{
			"n": "combination",
			"a": map[string]any{"type": r.params.GetDefault("combination", "subset"), "*": items},
		}},
	}
}

// --- CSV, TSV and wikitext tables ---

// cell renders one table cell; wikitext selects wiki links over plain text.
func cell(e *Entry, key string, number int, site *wiki.SiteInfo, wikitext, redlinks, thumbnails bool) string {
	switch key {
	case "number":
		return strconv.Itoa(number)
	case "title":
		if wikitext {
			return wikiLink(e, site, redlinks, thumbnails)
		}
		return e.Title.Underscored()
	case "page_id":
		return optInt64(e.PageID)
	case "namespace":
		if wikitext {
			return strconv.Itoa(e.Title.NS)
		}
		if name, ok := site.LocalName(e.Title.NS); ok {
			return name
		}
		return "UNKNOWN_NAMESPACE"
	case "size":
		if e.PageID == 0 {
			return ""
		}
		return strconv.FormatInt(e.Len, 10)
	case "timestamp":
		return e.Touched
	case "wikidata_item":
		if wikitext && e.WikidataItem != "" {
			return "[[:d:" + e.WikidataItem + "|]]"
		}
		return e.WikidataItem
	case "image":
		if wikitext && e.Image != "" {
			return "[[File:" + e.Image + "|120px|]]"
		}
		return e.Image
	case "defaultsort":
		return e.DefaultSort
	case "disambiguation":
		return map[triState]string{yes: "Y", no: "N"}[e.Disambiguation]
	case "incoming_links":
		return optCount(e.IncomingLinks)
	case "sitelinks":
		return optCount(e.SitelinkCount)
	case "redlink_count":
		return optCount(e.RedlinkCount)
	case "coordinates":
		if e.Coordinates == nil {
			return ""
		}
		return formatCoordinates(e.Coordinates)
	case "fileusage":
		return e.File.usageString(false)
	case "img_user_text":
		if !hasFileData(e) {
			return ""
		}
		if wikitext {
			return "[[User:" + e.File.User + "|]]"
		}
		return e.File.User
	}
	if v, ok := e.File.field(key); ok {
		return fmt.Sprint(v)
	}
	return ""
}

func optInt64(n int64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

func optCount(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

func (r *Run) renderTable(entries []*Entry, site *wiki.SiteInfo, format string) (*Response, error) {
	o := r.renderOptions()
	header := []column{
		{"number", "number"}, {"title", "title"}, {"page_id", "pageid"}, {"namespace", "namespace"},
		{"size", "length"}, {"timestamp", "touched"},
	}
	if o.showWikidataItem {
		header = append(header, column{"wikidata_item", "Wikidata"})
	}
	header = withColumns(header, o)

	rows := make([][]string, 0, len(entries)+1)
	labels := make([]string, len(header))
	for i, c := range header {
		labels[i] = c.label
	}
	rows = append(rows, labels)
	for n, e := range entries {
		row := make([]string, len(header))
		for i, c := range header {
			row[i] = cell(e, c.key, n+1, site, false, o.redlinks, false)
		}
		rows = append(rows, row)
	}

	var buf bytes.Buffer
	if format == "csv" {
		if err := csv.NewWriter(&buf).WriteAll(rows); err != nil {
			return nil, err
		}
		return &Response{Status: http.StatusOK, ContentType: "text/csv; charset=utf-8", Body: buf.Bytes()}, nil
	}
	for _, row := range rows {
		for i, v := range row {
			if i > 0 {
				buf.WriteByte('\t')
			}
			buf.WriteString(strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(v))
		}
		buf.WriteByte('\n')
	}
	return &Response{Status: http.StatusOK, ContentType: "text/tab-separated-values; charset=utf-8", Body: buf.Bytes()}, nil
}

func (r *Run) renderWiki(entries []*Entry, site *wiki.SiteInfo) *Response {
	o := r.renderOptions()
	header := []column{
		{"title", "Title"}, {"page_id", "Page ID"}, {"namespace", "Namespace"},
		{"size", "Size (bytes)"}, {"timestamp", "Last change"},
	}
	if o.showWikidataItem {
		header = append(header, column{"wikidata_item", "Wikidata"})
	}
	if o.redlinks {
		header = []column{{"redlink_count", "Wanted"}, {"title", "Title"}}
	}
	header = withColumns(header, o)
	thumbnails := r.params.Has("thumbnails_in_wiki_output")

	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", r.combination)
	fmt.Fprintf(&b, "Last updated on %s.\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "[%s Regenerate this table] or [%s edit the query].\n\n",
		r.queryURL(), strings.TrimSuffix(r.env.PublicURL, "/")+"/?"+r.params.Encode("doit", "format"))
	b.WriteString("{| border=1 class='wikitable'\n!")
	for i, c := range header {
		if i > 0 {
			b.WriteString(" !! ")
		}
		b.WriteString(c.label)
	}
	b.WriteByte('\n')
	for n, e := range entries {
		b.WriteString("|-\n| ")
		for i, c := range header {
			if i > 0 {
				b.WriteString(" || ")
			}
			b.WriteString(cell(e, c.key, n+1, site, true, o.redlinks, thumbnails))
		}
		b.WriteByte('\n')
	}
	b.WriteString("|}")
	return &Response{Status: http.StatusOK, ContentType: "text/plain; charset=utf-8", Body: []byte(b.String())}
}

// wikiLink renders the title cell of wikitext output.
func wikiLink(e *Entry, site *wiki.SiteInfo, redlinks, thumbnails bool) string {
	full, ok := site.FullPretty(e.Title)
	if !ok {
		full = e.Title.Name
	}
	switch {
	case site.Wiki == wikidataWiki:
		if e.WikidataLabel != "" {
			return "[[" + full + "|" + e.WikidataLabel + "]]"
		}
		return "[[" + full + "]]"
	case e.Title.NS == 6 && thumbnails:
		return "[[" + full + "|120px|]]"
	case e.Title.NS == 6:
		return "[[:" + full + "|]]"
	}
	link := "[["
	if e.Title.NS == 14 {
		link += ":"
	}
	link += full
	if !redlinks {
		link += "|"
	}
	return link + "]]"
}

// --- KML, plain text, PagePile ---

func (r *Run) renderKML(entries []*Entry, site *wiki.SiteInfo) *Response {
	var b strings.Builder
	esc := func(s string) string {
		var buf bytes.Buffer
		_ = xml.EscapeText(&buf, []byte(s))
		return buf.String()
	}
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<kml xmlns="http://www.opengis.net/kml/2.2"><Document>`)
	for _, e := range entries {
		if e.Coordinates == nil {
			continue
		}
		label := e.Title.Name
		if site.Wiki == wikidataWiki && e.WikidataLabel != "" {
			label = e.WikidataLabel
		}
		b.WriteString("<Placemark><name>" + esc(label) + "</name>")
		if e.WikidataDescription != "" {
			b.WriteString("<description>" + esc(e.WikidataDescription) + "</description>")
		}
		b.WriteString("<ExtendedData>")
		if e.WikidataItem != "" {
			b.WriteString(`<Data name="q"><value>` + esc(e.WikidataItem) + "</value></Data>")
		}
		full, _ := site.FullUnderscored(e.Title)
		b.WriteString(`<Data name="url"><value>` + esc(site.Server+"/wiki/"+url.PathEscape(full)) + "</value></Data>")
		if e.Image != "" {
			src := site.Server + "/wiki/Special:Redirect/file/" + url.PathEscape(e.Image) + "?width=120"
			b.WriteString(`<Data name="image"><value>` + esc(src) + "</value></Data>")
		}
		b.WriteString("</ExtendedData>")
		fmt.Fprintf(&b, "<Point><coordinates>%s,%s,0</coordinates></Point></Placemark>",
			strconv.FormatFloat(e.Coordinates.Lon, 'f', -1, 64), strconv.FormatFloat(e.Coordinates.Lat, 'f', -1, 64))
	}
	b.WriteString("</Document></kml>")
	return &Response{Status: http.StatusOK, ContentType: "application/vnd.google-earth.kml+xml", Body: []byte(b.String())}
}

func (r *Run) renderPlain(entries []*Entry, site *wiki.SiteInfo) *Response {
	var b strings.Builder
	for _, e := range entries {
		if full, ok := site.FullPretty(e.Title); ok {
			b.WriteString(full + "\n")
		}
	}
	return &Response{Status: http.StatusOK, ContentType: "text/plain; charset=utf-8", Body: []byte(b.String())}
}

// renderPagePile stores the result as a new PagePile and redirects to it.
func (r *Run) renderPagePile(ctx context.Context, entries []*Entry) (*Response, error) {
	var data strings.Builder
	for _, e := range entries {
		data.WriteString(e.Title.Name + "\t" + strconv.Itoa(e.Title.NS) + "\n")
	}
	form := url.Values{"action": {"create_pile_with_data"}, "wiki": {r.result.Wiki}, "data": {data.String()}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pagePileAPI, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var resp struct {
		Pile struct {
			ID int64 `json:"id"`
		} `json:"pile"`
	}
	if err := r.env.Wiki.Do(req, &resp); err != nil {
		return nil, upstream(fmt.Errorf("PagePile generation failed: %w", err))
	}
	if resp.Pile.ID == 0 {
		return nil, upstream(fmt.Errorf("PagePile generation did not return a PagePile ID"))
	}
	location := pagePileAPI + "?action=get_data&id=" + strconv.FormatInt(resp.Pile.ID, 10)
	return &Response{
		Status:      http.StatusSeeOther,
		ContentType: "text/plain; charset=utf-8",
		Header:      http.Header{"Location": {location}},
		Body:        []byte(location),
	}, nil
}
