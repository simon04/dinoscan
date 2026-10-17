package wiki

import (
	"encoding/json"
	"testing"
)

func testSite() *SiteInfo {
	return NewSiteInfo("dewiki", "https://de.wikipedia.org", []Namespace{
		{ID: 0, Name: "", Case: "first-letter"},
		{ID: 1, Name: "Diskussion", Canonical: "Talk", Case: "first-letter"},
		{ID: 14, Name: "Kategorie", Canonical: "Category", Case: "first-letter"},
		{ID: 2, Name: "Benutzer", Canonical: "User", Case: "first-letter"},
		{ID: 100, Name: "Portal", Canonical: "Portal", Case: "case-sensitive"},
	}, map[string]int{"Benutzerin": 2})
}

func TestParseTitle(t *testing.T) {
	site := testSite()
	tests := map[string]Title{
		"Berlin":                {0, "Berlin"},
		"berlin_mitte":          {0, "Berlin mitte"},
		"Kategorie:Stadt_in_X":  {14, "Stadt in X"},
		"category:stadt":        {14, "Stadt"},
		"Benutzerin:foo":        {2, "Foo"},
		"Portal:kleinschrift":   {100, "kleinschrift"},
		"Unbekannt:Foo":         {0, "Unbekannt:Foo"},
		"  Diskussion:Foo bar ": {1, "Foo bar"},
	}
	for in, want := range tests {
		if got := site.ParseTitle(in); got != want {
			t.Errorf("ParseTitle(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestFullTitles(t *testing.T) {
	site := testSite()
	if got, _ := site.FullPretty(NewTitle("Foo_bar", 14)); got != "Kategorie:Foo bar" {
		t.Errorf("FullPretty = %q", got)
	}
	if got, _ := site.FullUnderscored(NewTitle("Foo bar", 0)); got != "Foo_bar" {
		t.Errorf("FullUnderscored = %q", got)
	}
	if _, ok := site.FullPretty(NewTitle("Foo", 4711)); ok {
		t.Error("FullPretty of unknown namespace should fail")
	}
}

func TestUcFirst(t *testing.T) {
	for in, want := range map[string]string{"": "", "foo": "Foo", "über": "Über", "ǆ": "ǅ"} {
		if got := UcFirst(in); got != want {
			t.Errorf("UcFirst(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSiteMatrix(t *testing.T) {
	var raw map[string]json.RawMessage
	err := json.Unmarshal([]byte(`{
		"count": 3,
		"0": {"code": "de", "site": [
			{"url": "https://de.wikipedia.org", "dbname": "dewiki", "code": "wiki"},
			{"url": "https://de.wiktionary.org", "dbname": "dewiktionary", "code": "wiktionary"}
		]},
		"specials": [{"url": "https://commons.wikimedia.org", "dbname": "commonswiki", "code": "commons"}]
	}`), &raw)
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseSiteMatrix(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := m.ServerForWiki("dewiktionary"); s != "https://de.wiktionary.org" {
		t.Errorf("ServerForWiki(dewiktionary) = %q", s)
	}
	if s, _ := m.ServerForWiki("dewiktionarywiki"); s != "https://de.wiktionary.org" {
		t.Errorf("ServerForWiki(dewiktionarywiki) = %q", s)
	}
	for _, tt := range []struct{ lang, project, want string }{
		{"de", "wikipedia", "dewiki"},
		{"de", "wiktionary", "dewiktionary"},
		{"commons", "wikimedia", "commonswiki"},
		{"www", "wikidata", "wikidatawiki"},
	} {
		if got, _ := m.WikiForLanguageProject(tt.lang, tt.project); got != tt.want {
			t.Errorf("WikiForLanguageProject(%s, %s) = %q, want %q", tt.lang, tt.project, got, tt.want)
		}
	}
}
