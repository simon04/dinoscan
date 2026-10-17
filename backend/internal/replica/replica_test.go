package replica

import (
	"reflect"
	"testing"
)

func TestQuery(t *testing.T) {
	q := New("SELECT page_id FROM page WHERE page_namespace=?", 0).
		Add(" AND page_title").In(Strings([]string{"A", "B"})).
		Append(New(" AND page_len>=?", 100))
	if got, want := q.SQL(), "SELECT page_id FROM page WHERE page_namespace=? AND page_title IN (?,?) AND page_len>=?"; got != want {
		t.Errorf("SQL() = %q, want %q", got, want)
	}
	if got, want := q.Args(), []any{0, "A", "B", 100}; !reflect.DeepEqual(got, want) {
		t.Errorf("Args() = %v, want %v", got, want)
	}
}

func TestQueryPlaceholderMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic")
		}
	}()
	New("SELECT ?, ?", 1)
}

func TestQueryEmptyInPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic")
		}
	}()
	New("SELECT 1 FROM page WHERE page_id").In(nil)
}

func TestClusterFor(t *testing.T) {
	tests := []struct {
		wiki   string
		tables []string
		want   Cluster
		err    bool
	}{
		{"enwiki", []string{"page", "pagelinks"}, Core, false},
		{"commonswiki", []string{"page", "pagelinks"}, Links, false},
		{"commonswiki", []string{"page", "revision"}, Core, false},
		{"commonswiki", []string{"actor", "pagelinks"}, Core, true},
		{"wikidatawiki", []string{"wbt_item_terms"}, TermStore, false},
		{"wikidatawiki", []string{"page", "wb_items_per_site"}, Core, false},
	}
	for _, tt := range tests {
		got, err := ClusterFor(tt.wiki, tt.tables...)
		if (err != nil) != tt.err || (!tt.err && got != tt.want) {
			t.Errorf("ClusterFor(%s, %v) = %v, %v; want %v, error %v", tt.wiki, tt.tables, got, err, tt.want, tt.err)
		}
	}
}

func TestAddress(t *testing.T) {
	p := NewPools(Config{})
	if addr, db := p.address("enwiki", Core); addr != "enwiki.web.db.svc.wikimedia.cloud:3306" || db != "enwiki_p" {
		t.Errorf("address(enwiki) = %s, %s", addr, db)
	}
	if addr, _ := p.address("commonswiki", Links); addr != "links.commonswiki.web.db.svc.wikimedia.cloud:3306" {
		t.Errorf("address(commonswiki, Links) = %s", addr)
	}
	if addr, _ := p.address("wikidatawiki", TermStore); addr != "termstore.wikidatawiki.analytics.db.svc.wikimedia.cloud:3306" {
		t.Errorf("address(wikidatawiki, TermStore) = %s", addr)
	}
	local := NewPools(Config{Host: "127.0.0.1", Port: 3307, Ports: map[string]int{"links.commonswiki": 3315}})
	if addr, _ := local.address("commonswiki", Links); addr != "127.0.0.1:3315" {
		t.Errorf("local address(commonswiki, Links) = %s", addr)
	}
	if addr, _ := local.address("dewiki", Core); addr != "127.0.0.1:3307" {
		t.Errorf("local address(dewiki) = %s", addr)
	}
}

func TestFixWikiName(t *testing.T) {
	for in, want := range map[string]string{"be-taraskwiki": "be_x_oldwiki", "zh-min-nanwiki": "zh_min_nanwiki", "enwiki": "enwiki"} {
		if got := FixWikiName(in); got != want {
			t.Errorf("FixWikiName(%s) = %s, want %s", in, got, want)
		}
	}
}
