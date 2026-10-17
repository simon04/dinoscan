package wiki

import (
	"sort"
	"strings"
)

// Namespace is one namespace of a wiki, from the siteinfo API.
type Namespace struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Canonical string `json:"canonical"`
	Case      string `json:"case"`
}

// SiteInfo holds what is needed about one wiki: its server and namespaces.
type SiteInfo struct {
	Wiki       string
	Server     string // e.g. https://en.wikipedia.org
	Namespaces map[int]Namespace
	// byName maps lower-cased local, canonical and alias names, with spaces,
	// to namespace IDs.
	byName map[string]int
}

// NewSiteInfo indexes namespaces and their aliases.
func NewSiteInfo(wiki, server string, namespaces []Namespace, aliases map[string]int) *SiteInfo {
	s := &SiteInfo{
		Wiki:       wiki,
		Server:     server,
		Namespaces: map[int]Namespace{},
		byName:     map[string]int{},
	}
	for alias, id := range aliases {
		s.byName[normalizeNamespaceName(alias)] = id
	}
	for _, ns := range namespaces {
		s.Namespaces[ns.ID] = ns
		if ns.Canonical != "" {
			s.byName[normalizeNamespaceName(ns.Canonical)] = ns.ID
		}
		if ns.Name != "" {
			s.byName[normalizeNamespaceName(ns.Name)] = ns.ID
		}
	}
	return s
}

func normalizeNamespaceName(s string) string {
	return strings.ToLower(UnderscoresToSpaces(s))
}

// ParseTitle splits a namespace-prefixed title, e.g. "Category:Foo_bar".
// A prefix that is no namespace of this wiki stays part of the name.
func (s *SiteInfo) ParseTitle(full string) Title {
	full = UnderscoresToSpaces(full)
	ns, name := 0, full
	if prefix, rest, found := strings.Cut(full, ":"); found {
		if id, ok := s.byName[normalizeNamespaceName(prefix)]; ok {
			ns, name = id, UnderscoresToSpaces(rest)
		}
	}
	if !s.CaseSensitive(ns) {
		name = UcFirst(name)
	}
	return Title{NS: ns, Name: name}
}

// CaseSensitive reports whether titles in namespace ns are case-sensitive in
// their first letter too.
func (s *SiteInfo) CaseSensitive(ns int) bool {
	return s.Namespaces[ns].Case == "case-sensitive"
}

// LocalName returns the local name of namespace ns, e.g. "Kategorie" on
// dewiki; false if the wiki has no such namespace.
func (s *SiteInfo) LocalName(ns int) (string, bool) {
	n, ok := s.Namespaces[ns]
	return n.Name, ok
}

// CanonicalName returns the canonical (English) name of namespace ns.
func (s *SiteInfo) CanonicalName(ns int) string {
	return s.Namespaces[ns].Canonical
}

// FullPretty returns the prefixed title with spaces, e.g. "Category:Foo bar".
func (s *SiteInfo) FullPretty(t Title) (string, bool) {
	prefix, ok := s.LocalName(t.NS)
	if !ok {
		return "", false
	}
	if prefix == "" {
		return t.Name, true
	}
	return prefix + ":" + t.Name, true
}

// FullUnderscored returns the prefixed title with underscores.
func (s *SiteInfo) FullUnderscored(t Title) (string, bool) {
	full, ok := s.FullPretty(t)
	return SpacesToUnderscores(full), ok
}

// SortedNamespaceIDs returns the namespace IDs in ascending order.
func (s *SiteInfo) SortedNamespaceIDs() []int {
	ids := make([]int, 0, len(s.Namespaces))
	for id := range s.Namespaces {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}
