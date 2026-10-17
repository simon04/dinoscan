// Package petscan runs PetScan queries: it gathers pages from data sources,
// combines and filters them, and renders the result.
//
// The query parameters are those of PetScan, see
// https://meta.wikimedia.org/wiki/PetScan/en
package petscan

import (
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// Params are the query parameters of one PetScan query.
type Params struct {
	values map[string]string
	// Namespaces are the IDs of the ns[…] parameters that are set.
	Namespaces []int
}

var reNamespaceParam = regexp.MustCompile(`^ns\[(\d+)\]$`)

// ParseParams parses a URL query string. For repeated keys, the last value
// wins.
func ParseParams(query string) (Params, error) {
	parsed, err := url.ParseQuery(query)
	if err != nil {
		return Params{}, err
	}
	values := make(map[string]string, len(parsed))
	for k, v := range parsed {
		values[k] = v[len(v)-1]
	}
	return NewParams(values), nil
}

// NewParams wraps key/value pairs and applies the legacy parameter aliases.
func NewParams(values map[string]string) Params {
	p := Params{values: values}
	p.applyLegacy()
	p.Namespaces = nil
	for k, v := range p.values {
		m := reNamespaceParam.FindStringSubmatch(k)
		if m == nil || !isTruthy(v) {
			continue
		}
		if ns, err := strconv.Atoi(m[1]); err == nil {
			p.Namespaces = append(p.Namespaces, ns)
		}
	}
	sort.Ints(p.Namespaces)
	return p
}

// isTruthy accepts "1" (PetScan's form) as well as "true", "on" etc.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "off", "no":
		return false
	}
	return true
}

// applyLegacy maps parameters of older PetScan, CatScan and QuickIntersection
// versions to the current ones.
func (p *Params) applyLegacy() {
	p.fallback("language", "lang")
	p.fallback("categories", "cats")
	p.fallback("rxp_filter", "regexp_filter")

	// Old hack using a manual wiki with no pages as "common wiki".
	if p.Has("manual_list_wiki") && !p.Has("manual_list") && !p.Has("common_wiki_other") {
		p.values["common_wiki_other"] = p.values["manual_list_wiki"]
		p.values["manual_list_wiki"] = ""
	}

	// Query originally from QuickIntersection.
	if _, ok := p.values["max"]; ok {
		if p.values["format"] == "jsonfm" {
			p.values["json-pretty"] = "1"
		}
		p.values["output_compatability"] = "quick-intersection"
		if ns, ok := p.values["ns"]; ok {
			if ns == "*" {
				p.values["ns[0]"] = "1"
			} else if n, err := strconv.Atoi(ns); err == nil {
				p.values["ns["+strconv.Itoa(n)+"]"] = "1"
			}
		}
	}

	if _, ok := p.values["comb_subset"]; ok {
		p.values["combination"] = "subset"
	}
	if _, ok := p.values["comb_union"]; ok {
		p.values["combination"] = "union"
	}
	for _, key := range []string{"get_q", "wikidata"} {
		if _, ok := p.values[key]; ok {
			p.values["wikidata_item"] = "any"
		}
	}
	if _, ok := p.values["wikidata_no_item"]; ok {
		p.values["wikidata_item"] = "without"
	}
}

// fallback sets primary to the value of legacy if primary is unset or empty.
func (p *Params) fallback(primary, legacy string) {
	if v, ok := p.values[legacy]; ok && p.values[primary] == "" {
		p.values[primary] = v
	}
}

// Has reports whether key is set to a non-empty value.
func (p Params) Has(key string) bool {
	return p.values[key] != ""
}

// Get returns the value of key, or "".
func (p Params) Get(key string) string {
	return p.values[key]
}

// GetDefault returns the value of key, or def if it is unset or empty.
func (p Params) GetDefault(key, def string) string {
	if v := p.values[key]; v != "" {
		return v
	}
	return def
}

// Int returns key as a non-negative integer, if it is one.
func (p Params) Int(key string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(p.values[key]))
	return n, err == nil && n >= 0
}

// List splits key at sep into trimmed, non-empty titles with underscores.
func (p Params) List(key, sep string) []string {
	var ret []string
	for _, s := range strings.Split(p.values[key], sep) {
		s = strings.Trim(strings.TrimSpace(s), "‎‏")
		if s != "" {
			ret = append(ret, wiki.SpacesToUnderscores(s))
		}
	}
	return ret
}

// Encode returns the parameters as a query string with sorted keys,
// optionally without some keys.
func (p Params) Encode(without ...string) string {
	values := url.Values{}
	for k, v := range p.values {
		if !slices.Contains(without, k) {
			values.Set(k, v)
		}
	}
	return values.Encode()
}

// Format returns the output format; unknown formats are returned as is and
// rejected by the renderer.
func (p Params) Format() string {
	return p.GetDefault("format", "html")
}

// MainWiki returns the database name of the wiki given by the language and
// project parameters, e.g. "enwiki".
func (p Params) MainWiki(matrix *wiki.SiteMatrix) (string, bool) {
	language := strings.ReplaceAll(p.GetDefault("language", "en"), "_", "-")
	project := p.GetDefault("project", "wikipedia")
	w, ok := matrix.WikiForLanguageProject(language, project)
	if !ok {
		return "", false
	}
	return replica.FixWikiName(w), true
}
