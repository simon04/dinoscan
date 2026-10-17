package petscan

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/simon04/dinoscan/backend/internal/wiki"
)

func TestParseParams(t *testing.T) {
	p, err := ParseParams("lang=de&cats=A%0AB+c&ns%5B0%5D=1&ns%5B14%5D=true&ns%5B6%5D=0&depth=2&comb_union=1")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Get("language"); got != "de" {
		t.Errorf("language = %q", got)
	}
	if got, want := p.List("categories", "\n"), []string{"A", "B_c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("categories = %v, want %v", got, want)
	}
	if got, want := p.Namespaces, []int{0, 14}; !reflect.DeepEqual(got, want) {
		t.Errorf("Namespaces = %v, want %v", got, want)
	}
	if got := p.Get("combination"); got != "union" {
		t.Errorf("combination = %q", got)
	}
	if n, ok := p.Int("depth"); !ok || n != 2 {
		t.Errorf("depth = %d, %v", n, ok)
	}
}

func TestParseParamsQuickIntersection(t *testing.T) {
	p, _ := ParseParams("max=10&ns=*&format=jsonfm")
	if p.Get("output_compatability") != "quick-intersection" || p.Get("json-pretty") != "1" {
		t.Errorf("legacy QuickIntersection parameters not mapped: %v", p.values)
	}
	if !reflect.DeepEqual(p.Namespaces, []int{0}) {
		t.Errorf("Namespaces = %v", p.Namespaces)
	}
}

func TestParseCombination(t *testing.T) {
	tests := map[string]string{
		"":                                  "nothing",
		"categories":                        "categories",
		"Categories":                        "categories",
		"categories AND sparql":             "(categories AND sparql)",
		"categories and sparql or manual":   "(categories AND (sparql OR manual))",
		"(categories or sparql) not manual": "((categories OR sparql) NOT manual)",
		"(categories or sparql":             "nothing",
		"categories xor sparql":             "nothing",
	}
	for in, want := range tests {
		if got := parseCombination(in).String(); got != want {
			t.Errorf("parseCombination(%q) = %s, want %s", in, got, want)
		}
	}
	if got := defaultCombination([]string{"categories", "sparql", "manual"}).String(); got != "(manual AND (sparql AND categories))" {
		t.Errorf("defaultCombination = %s", got)
	}
}

func pageList(w string, titles ...string) *PageList {
	pl := NewPageList(w)
	for _, t := range titles {
		pl.AddTitle(wiki.NewTitle(t, 0))
	}
	return pl
}

func names(pl *PageList) []string {
	var ret []string
	for _, e := range pl.Entries() {
		ret = append(ret, e.Title.Name)
	}
	slices.Sort(ret)
	return ret
}

func TestCombine(t *testing.T) {
	r := &Run{}
	results := map[string]*PageList{
		"categories": pageList("enwiki", "A", "B", "C"),
		"manual":     pageList("enwiki", "B", "C", "D"),
		"search":     pageList("enwiki", "C"),
	}
	got, err := r.combine(t.Context(), parseCombination("categories and manual not search"), results)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"B"}; !reflect.DeepEqual(names(got), want) {
		t.Errorf("combine = %v, want %v", names(got), want)
	}
	if _, err := r.combine(t.Context(), parseCombination("categories"), results); err == nil {
		t.Error("using a consumed source should fail")
	}
}

func TestSortEntries(t *testing.T) {
	one, three := 1, 3
	entries := []*Entry{
		{Title: wiki.NewTitle("B", 0), PageID: 2, RedlinkCount: &one},
		{Title: wiki.NewTitle("A", 0), PageID: 3},
		{Title: wiki.NewTitle("C", 14), PageID: 1, RedlinkCount: &three},
	}
	titles := func() string {
		var s []string
		for _, e := range entries {
			s = append(s, e.Title.Name)
		}
		return strings.Join(s, ",")
	}
	for _, tt := range []struct {
		by   string
		desc bool
		want string
	}{
		{"", false, "C,B,A"},
		{"title", false, "A,B,C"},
		{"title", true, "C,B,A"},
		{"ns_title", true, "C,B,A"},
		{"redlinks", false, "B,C,A"},
		{"redlinks", true, "A,C,B"},
	} {
		sortEntries(entries, tt.by, tt.desc, false)
		if got := titles(); got != tt.want {
			t.Errorf("sortEntries(%q, %v) = %s, want %s", tt.by, tt.desc, got, tt.want)
		}
	}
}

func TestCategoryBatches(t *testing.T) {
	groups := [][]string{make([]string, 25001), {"x", "y"}}
	batches := categoryBatches(groups, maxPlaceholders)
	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2", len(batches))
	}
	for _, b := range batches {
		if len(b) != 2 || len(b[1]) != 2 {
			t.Errorf("batch has wrong shape: %d groups", len(b))
		}
	}
	// Three groups must stay within the placeholder limit together.
	big := make([]string, 50000)
	for _, b := range categoryBatches([][]string{big, big, big}, maxPlaceholders-15000) {
		n := 0
		for _, g := range b {
			n += len(g)
		}
		if n > maxPlaceholders-15000 {
			t.Errorf("batch has %d placeholders", n)
		}
	}
}

func TestParseSPARQL(t *testing.T) {
	body := []byte(`{"head":{"vars":["item","label"]},"results":{"bindings":[
		{"item":{"type":"uri","value":"http://www.wikidata.org/entity/Q42"},"label":{"value":"Douglas` + "\x01" + `"}},
		{"item":{"type":"uri","value":"http://www.wikidata.org/entity/P31"}},
		{"label":{"value":"no item"}}
	]}}`)
	pl, err := parseSPARQL(body, 10)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Get(wiki.Title{NS: 0, Name: "Q42"}) == nil || pl.Get(wiki.Title{NS: 120, Name: "P31"}) == nil || pl.Len() != 2 {
		t.Errorf("parseSPARQL = %v", pl.Entries())
	}
	if _, err := parseSPARQL(body, 1); err == nil {
		t.Error("expected too many results")
	}
}

func TestTemplateClause(t *testing.T) {
	q := templateClause([]string{"Infobox"}, true, true)
	if !strings.HasPrefix(q.SQL(), " AND NOT EXISTS (SELECT 1 FROM page pt,templatelinks") || len(q.Args()) != 1 {
		t.Errorf("templateClause = %q %v", q.SQL(), q.Args())
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`100%_a\b`); got != `100\%\_a\\b` {
		t.Errorf("escapeLike = %s", got)
	}
}

func TestIsJSIdentifier(t *testing.T) {
	if !isJSIdentifier("jQuery_123.cb") || isJSIdentifier("alert(1)") || isJSIdentifier("") {
		t.Error("isJSIdentifier")
	}
}
