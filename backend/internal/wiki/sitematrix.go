package wiki

import (
	"encoding/json"
	"strings"
)

// SiteMatrix maps between wiki database names and server URLs.
type SiteMatrix struct {
	serverByWiki map[string]string
	wikiByServer map[string]string
}

type siteMatrixSite struct {
	URL    string `json:"url"`
	DBName string `json:"dbname"`
}

// parseSiteMatrix reads the "sitematrix" member of an action=sitematrix
// response (format version 2): numbered language groups with a "site" list,
// plus a "specials" list and a "count".
func parseSiteMatrix(raw map[string]json.RawMessage) (*SiteMatrix, error) {
	m := &SiteMatrix{serverByWiki: map[string]string{}, wikiByServer: map[string]string{}}
	add := func(sites []siteMatrixSite) {
		for _, s := range sites {
			m.serverByWiki[s.DBName] = s.URL
			m.wikiByServer[s.URL] = s.DBName
		}
	}
	for key, value := range raw {
		switch key {
		case "count":
		case "specials":
			var sites []siteMatrixSite
			if err := json.Unmarshal(value, &sites); err != nil {
				return nil, err
			}
			add(sites)
		default:
			var group struct {
				Site []siteMatrixSite `json:"site"`
			}
			if err := json.Unmarshal(value, &group); err != nil {
				return nil, err
			}
			add(group.Site)
		}
	}
	return m, nil
}

// NewSiteMatrix builds a site matrix from wiki to server pairs (for tests).
func NewSiteMatrix(serverByWiki map[string]string) *SiteMatrix {
	m := &SiteMatrix{serverByWiki: map[string]string{}, wikiByServer: map[string]string{}}
	for wiki, server := range serverByWiki {
		m.serverByWiki[wiki] = server
		m.wikiByServer[server] = wiki
	}
	return m
}

// ServerForWiki returns e.g. https://en.wikipedia.org for enwiki.
func (m *SiteMatrix) ServerForWiki(wiki string) (string, bool) {
	switch strings.ReplaceAll(wiki, "_", "-") {
	case "be-taraskwiki", "be-x-oldwiki":
		return "https://be-tarask.wikipedia.org", true
	case "metawiki":
		return "https://meta.wikimedia.org", true
	}
	if server, ok := m.serverByWiki[wiki]; ok {
		return server, true
	}
	// Tolerate names like "enwiktionarywiki" for enwiktionary.
	if trimmed, ok := strings.CutSuffix(wiki, "wiki"); ok && strings.Contains(trimmed, "wik") {
		server, ok := m.serverByWiki[trimmed]
		return server, ok
	}
	return "", false
}

// WikiForServer returns e.g. enwiki for https://en.wikipedia.org.
func (m *SiteMatrix) WikiForServer(server string) (string, bool) {
	wiki, ok := m.wikiByServer[server]
	return wiki, ok
}

// WikiForLanguageProject returns the database name of e.g. ("de",
// "wikipedia"). "commons" and "wikidata" are accepted as either part.
func (m *SiteMatrix) WikiForLanguageProject(language, project string) (string, bool) {
	switch {
	case project == "wikipedia":
		return language + "wiki", true
	case language == "commons":
		return "commonswiki", true
	case language == "wikidata", project == "wikidata":
		return "wikidatawiki", true
	}
	return m.WikiForServer("https://" + language + "." + project + ".org")
}
