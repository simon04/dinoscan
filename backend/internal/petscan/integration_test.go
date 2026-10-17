package petscan

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// The integration tests run queries against a MariaDB server holding a tiny
// replica-like fixture, so that all generated SQL is executed for real. They
// are skipped unless DINOSCAN_TEST_MARIADB is set to host:port of a server
// accepting root without password; scripts/test-mariadb.sh starts one.

const fixtureSchema = `
CREATE TABLE page (page_id INT PRIMARY KEY, page_namespace INT, page_title VARBINARY(255),
  page_is_redirect TINYINT DEFAULT 0, page_latest INT, page_len INT, UNIQUE KEY (page_namespace, page_title));
CREATE TABLE revision (rev_id INT PRIMARY KEY, rev_page INT, rev_timestamp BINARY(14), rev_parent_id INT,
  rev_actor INT, rev_len INT);
CREATE TABLE actor (actor_id INT PRIMARY KEY, actor_user INT NULL, actor_name VARBINARY(255));
CREATE TABLE user_groups (ug_user INT, ug_group VARBINARY(255));
CREATE TABLE linktarget (lt_id INT PRIMARY KEY, lt_namespace INT, lt_title VARBINARY(255));
CREATE TABLE categorylinks (cl_from INT, cl_target_id INT, cl_type ENUM('page','subcat','file'));
CREATE TABLE templatelinks (tl_from INT, tl_from_namespace INT, tl_target_id INT);
CREATE TABLE pagelinks (pl_from INT, pl_from_namespace INT, pl_target_id INT);
CREATE TABLE page_props (pp_page INT, pp_propname VARBINARY(60), pp_value BLOB, pp_sortkey FLOAT NULL);
CREATE TABLE flaggedpages (fp_page_id INT, fp_pending_since BINARY(14) NULL);
CREATE TABLE langlinks (ll_from INT, ll_lang VARBINARY(35), ll_title VARBINARY(255));
CREATE TABLE geo_tags (gt_page_id INT, gt_primary TINYINT, gt_globe VARBINARY(32), gt_lat DECIMAL(11,8), gt_lon DECIMAL(11,8));
CREATE TABLE globalimagelinks (gil_wiki VARBINARY(32), gil_page INT, gil_page_namespace_id INT,
  gil_page_namespace VARBINARY(255), gil_page_title VARBINARY(255), gil_to VARBINARY(255));
CREATE TABLE image_compat (img_name VARBINARY(255), img_size INT, img_width INT, img_height INT,
  img_media_type VARBINARY(32), img_major_mime VARBINARY(32), img_minor_mime VARBINARY(100),
  img_user_text VARBINARY(255), img_timestamp BINARY(14), img_sha1 VARBINARY(32));
CREATE TABLE ores_classification (oresc_rev INT, oresc_model INT, oresc_probability DECIMAL(3,3), oresc_is_predicted TINYINT);
CREATE TABLE ores_model (oresm_id INT, oresm_name VARBINARY(32), oresm_is_current TINYINT);
CREATE TABLE wb_items_per_site (ips_item_id INT, ips_site_id VARBINARY(32), ips_site_page VARBINARY(310));
CREATE TABLE wbt_item_terms (wbit_item_id INT, wbit_term_in_lang_id INT);
CREATE TABLE wbt_property_terms (wbpt_property_id INT, wbpt_term_in_lang_id INT);
CREATE TABLE wbt_term_in_lang (wbtl_id INT, wbtl_type_id INT, wbtl_text_in_lang_id INT);
CREATE TABLE wbt_text_in_lang (wbxl_id INT, wbxl_language VARBINARY(20), wbxl_text_id INT);
CREATE TABLE wbt_text (wbx_id INT, wbx_text VARBINARY(255));
`

const fixtureEnwiki = `
INSERT INTO page VALUES
  (1,0,'Lion',0,11,1000),(2,0,'Tiger',0,21,500),(3,0,'Ghost',0,31,10),(4,1,'Lion',0,41,20),
  (5,6,'Lion.jpg',0,51,0),(6,0,'Lion/Sub',0,61,5),
  (100,14,'Animals',0,1001,0),(101,14,'Cats',0,1011,0),(102,14,'Hidden_cat',0,1021,0);
INSERT INTO revision VALUES (11,1,'20260101000000',0,1,1000),(21,2,'20250101000000',0,2,400),
  (31,3,'20240101000000',0,1,10),(41,4,'20260101000000',0,1,20),(51,5,'20260101000000',0,1,0),
  (61,6,'20260101000000',0,1,5);
INSERT INTO actor VALUES (1,1,'Alice Example'),(2,NULL,'192.0.2.1');
INSERT INTO linktarget VALUES (1,14,'Animals'),(2,14,'Cats'),(3,14,'Hidden_cat'),(4,10,'Infobox'),
  (5,0,'Tiger'),(6,0,'Missing'),(7,0,'Lion');
INSERT INTO categorylinks VALUES (1,1,'page'),(101,1,'subcat'),(102,1,'subcat'),(2,2,'page'),(3,3,'page'),(5,1,'file');
INSERT INTO templatelinks VALUES (1,0,4);
INSERT INTO pagelinks VALUES (1,0,5),(1,0,6),(2,0,7);
INSERT INTO page_props VALUES (1,'wikibase_item','Q1',NULL),(2,'wikibase_item','Q2',NULL),
  (102,'hiddencat','',NULL),(1,'page_image_free','Lion.jpg',NULL),(1,'defaultsort','Lion, The',NULL);
INSERT INTO geo_tags VALUES (1,1,'earth',-2.33333300,34.83333300);
INSERT INTO langlinks VALUES (1,'de','Löwe');
INSERT INTO globalimagelinks VALUES ('dewiki',7,0,'','Löwe','Lion.jpg');
INSERT INTO image_compat VALUES ('Lion.jpg',12345,800,600,'BITMAP','image','jpeg','Alice Example','20200101000000','abc');
`

const fixtureWikidata = `
INSERT INTO page VALUES (1,0,'Q1',0,1,100),(2,0,'Q2',0,2,100),(3,120,'P31',0,3,10);
INSERT INTO revision VALUES (1,1,'20260101000000',0,1,100),(2,2,'20260101000000',0,1,100),(3,3,'20260101000000',0,1,10);
INSERT INTO actor VALUES (1,1,'Alice Example');
INSERT INTO page_props VALUES (1,'wb-claims','3',3),(2,'wb-claims','0',0);
INSERT INTO linktarget VALUES (1,120,'P31');
INSERT INTO pagelinks VALUES (1,0,1);
INSERT INTO wb_items_per_site VALUES (1,'enwiki','Lion'),(1,'dewiki','Löwe'),(2,'enwiki','Tiger');
INSERT INTO wbt_item_terms VALUES (1,1),(2,2),(1,3);
INSERT INTO wbt_term_in_lang VALUES (1,1,1),(2,1,2),(3,2,3);
INSERT INTO wbt_text_in_lang VALUES (1,'en',1),(2,'en',2),(3,'en',3);
INSERT INTO wbt_text VALUES (1,'lion'),(2,'tiger'),(3,'big cat');
`

func setupFixture(t *testing.T) *Env {
	addr := os.Getenv("DINOSCAN_TEST_MARIADB")
	if addr == "" {
		t.Skip("DINOSCAN_TEST_MARIADB not set; see scripts/test-mariadb.sh")
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Net, cfg.Addr, cfg.MultiStatements = "root", "tcp", addr, true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for name, data := range map[string]string{
		"enwiki_p": fixtureEnwiki, "commonswiki_p": fixtureEnwiki, "dewiki_p": "", "wikidatawiki_p": fixtureWikidata,
	} {
		script := "DROP DATABASE IF EXISTS " + name + "; CREATE DATABASE " + name + "; USE " + name + ";" + fixtureSchema + data
		if _, err := db.Exec(script); err != nil {
			t.Fatalf("fixture %s: %v", name, err)
		}
	}

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	pools := replica.NewPools(replica.Config{User: "root", Host: host, Port: port})
	t.Cleanup(pools.Close)

	api := httptest.NewServer(http.HandlerFunc(fakeAPI))
	t.Cleanup(api.Close)
	client := wiki.NewClient()
	client.SiteMatrixURL = api.URL + "/meta/w/api.php"
	fakeAPIBase = api.URL
	return &Env{DB: pools, Wiki: client, MaxResults: 1000, PublicURL: "https://dinoscan.example/"}
}

var fakeAPIBase string

// fakeAPI answers the few MediaWiki API requests the backend makes.
func fakeAPI(w http.ResponseWriter, req *http.Request) {
	_ = req.ParseForm()
	w.Header().Set("Content-Type", "application/json")
	site := func(dbname string) map[string]string {
		return map[string]string{"url": fakeAPIBase + "/" + dbname, "dbname": dbname}
	}
	switch {
	case req.Form.Get("action") == "sitematrix":
		_ = json.NewEncoder(w).Encode(map[string]any{"sitematrix": map[string]any{
			"count":    3,
			"0":        map[string]any{"code": "en", "site": []any{site("enwiki")}},
			"1":        map[string]any{"code": "de", "site": []any{site("dewiki")}},
			"specials": []any{site("wikidatawiki"), site("commonswiki")},
		}})
	case req.Form.Get("meta") == "siteinfo":
		ns := []map[string]any{{"id": 0, "name": "", "case": "first-letter"}}
		switch strings.Split(req.URL.Path, "/")[1] {
		case "wikidatawiki":
			ns = append(ns, map[string]any{"id": 120, "name": "Property", "canonical": "Property", "case": "first-letter"})
		case "dewiki":
			ns = append(ns, map[string]any{"id": 14, "name": "Kategorie", "canonical": "Category", "case": "first-letter"})
		default:
			for id, name := range map[int]string{1: "Talk", 2: "User", 6: "File", 10: "Template", 14: "Category"} {
				ns = append(ns, map[string]any{"id": id, "name": name, "canonical": name, "case": "first-letter"})
			}
		}
		namespaces := map[string]any{}
		for _, n := range ns {
			namespaces[strconv.Itoa(n["id"].(int))] = n
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"namespaces": namespaces, "namespacealiases": []any{}}})
	case req.Form.Get("list") == "search":
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"search": []any{
			map[string]any{"ns": 0, "title": "Tiger", "pageid": 2},
		}}})
	default:
		http.Error(w, "unexpected request "+req.Form.Encode(), http.StatusBadRequest)
	}
}

// query runs a query and returns the rendered response.
func query(t *testing.T, env *Env, q string) *Response {
	t.Helper()
	params, err := ParseParams(q)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRun(env, params)
	if err := r.Execute(t.Context()); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	resp, err := r.Render(t.Context())
	if err != nil {
		t.Fatalf("%s: render: %v", q, err)
	}
	return resp
}

type jsonPage struct {
	Title     string         `json:"title"`
	ID        int64          `json:"id"`
	Namespace int            `json:"namespace"`
	Len       int64          `json:"len"`
	Touched   string         `json:"touched"`
	Q         string         `json:"q"`
	Metadata  map[string]any `json:"metadata"`
}

// pages runs a JSON query and returns the result pages.
func pages(t *testing.T, env *Env, q string) []jsonPage {
	t.Helper()
	resp := query(t, env, q+"&format=json&doit=1")
	var result struct {
		Star []struct {
			A struct {
				Star []jsonPage `json:"*"`
			} `json:"a"`
		} `json:"*"`
	}
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("%s: %v: %s", q, err, resp.Body)
	}
	return result.Star[0].A.Star
}

func titles(ps []jsonPage) string {
	var ret []string
	for _, p := range ps {
		ret = append(ret, p.Title)
	}
	slices.Sort(ret)
	return strings.Join(ret, ",")
}

func TestIntegration(t *testing.T) {
	env := setupFixture(t)
	tests := []struct {
		name, query, want string
	}{
		{"category", "language=en&project=wikipedia&categories=Animals&ns%5B0%5D=1", "Lion"},
		{"category tree", "language=en&categories=Animals&depth=1&ns%5B0%5D=1", "Ghost,Lion,Tiger"},
		{"skip hidden categories", "language=en&categories=Animals&depth=1&ns%5B0%5D=1&skip_hidden_categories=1", "Lion,Tiger"},
		{"category intersection", "language=en&categories=Animals%0ACats&depth=0&combination=subset", ""},
		{"category union", "language=en&categories=Animals%0ACats&combination=union&ns%5B0%5D=1", "Lion,Tiger"},
		{"negative category", "language=en&categories=Animals&depth=1&negcats=Cats&ns%5B0%5D=1", "Ghost,Lion"},
		{"template", "language=en&templates_yes=infobox&ns%5B0%5D=1", "Lion"},
		{"template negative", "language=en&categories=Animals&depth=1&templates_no=Infobox&ns%5B0%5D=1", "Ghost,Tiger"},
		{"template on talk page", "language=en&categories=Animals&depth=1&templates_any=Infobox&templates_use_talk_any=1&ns%5B0%5D=1", ""},
		{"links to", "language=en&links_to_all=Tiger&ns%5B0%5D=1", "Lion"},
		{"links from", "language=en&outlinks_yes=Lion&ns%5B0%5D=1", "Tiger"},
		{"links to several namespaces", "language=en&links_to_any=Tiger%0ACategory:Nope&ns%5B0%5D=1", "Lion"},
		{"links from several namespaces", "language=en&outlinks_any=Lion%0ATalk:Lion&ns%5B0%5D=1", "Tiger"},
		{"links to none", "language=en&categories=Animals&depth=1&links_to_no=Lion&ns%5B0%5D=1", "Ghost,Lion"},
		{"size", "language=en&categories=Animals&depth=1&larger=100&ns%5B0%5D=1", "Lion,Tiger"},
		{"min links", "language=en&categories=Animals&depth=1&minlinks=2&ns%5B0%5D=1", "Lion"},
		{"last edited after", "language=en&categories=Animals&depth=1&after=20250601000000&ns%5B0%5D=1", "Lion"},
		{"anonymous last edit", "language=en&categories=Animals&depth=1&edits%5Banons%5D=yes&ns%5B0%5D=1", "Tiger"},
		{"created by", "language=en&created_by=Alice_Example&ns%5B0%5D=1", "Ghost,Lion,Lion/Sub"},
		{"talk page exists", "language=en&categories=Animals&depth=1&talk_page_exists=yes&ns%5B0%5D=1", "Lion"},
		{"page image", "language=en&categories=Animals&depth=1&page_image=no&ns%5B0%5D=1", "Ghost,Tiger"},
		{"no subpages", "language=en&created_by=Alice_Example&subpage_filter=no_subpages&ns%5B0%5D=1", "Ghost,Lion"},
		{"manual list", "manual_list=Lion%0Atalk:Lion%0ANope&manual_list_wiki=enwiki", "Lion,Lion"},
		{"manual list with database filter", "manual_list=Lion%0ATiger&manual_list_wiki=enwiki&templates_no=Infobox", "Tiger"},
		{"search", "search_query=stripes&search_wiki=enwiki&search_max_results=10", "Tiger"},
		{"with wikidata item", "language=en&categories=Animals&depth=1&wikidata_item=with&ns%5B0%5D=1", "Lion,Tiger"},
		{"without wikidata item", "language=en&categories=Animals&depth=1&wikidata_item=without&ns%5B0%5D=1", "Ghost"},
		{"to wikidata", "language=en&categories=Animals&depth=1&common_wiki=wikidata&ns%5B0%5D=1", "Q1,Q2"},
		{"to other wiki", "language=en&categories=Animals&depth=1&common_wiki=other&common_wiki_other=dewiki&ns%5B0%5D=1", "Löwe"},
		{"sitelinks", "language=en&categories=Animals&depth=1&sitelinks_yes=dewiki&ns%5B0%5D=1", "Lion"},
		{"sitelinks source", "sitelinks_yes=enwiki&sitelinks_any=dewiki", "Lion"},
		{"statements", "language=en&categories=Animals&depth=1&min_statements=1&ns%5B0%5D=1", "Lion"},
		{"uses property", "language=en&categories=Animals&depth=1&wikidata_prop_item_use=P31&wpiu=any&ns%5B0%5D=1", "Lion"},
		{"labels", "language=en&categories=Animals&depth=1&labels_yes=ti%25&langs_labels_yes=en&cb_labels_yes_l=1&ns%5B0%5D=1", "Q2"},
		{"labels none", "language=en&categories=Animals&depth=1&labels_no=big_cat&cb_labels_no_d=1&ns%5B0%5D=1", "Q2"},
		{"regexp", "language=en&categories=Animals&depth=1&rxp_filter=.i.*&ns%5B0%5D=1", "Lion,Tiger"},
		{"talk pages", "manual_list=Lion&manual_list_wiki=enwiki&namespace_conversion=talk", "Lion"},
		{"redlinks", "language=en&categories=Animals&depth=1&show_redlinks=1&ns%5B0%5D=1", "Missing"},
		{"combination", "language=en&categories=Animals&depth=1&manual_list=Tiger&manual_list_wiki=enwiki&source_combination=categories+not+manual&ns%5B0%5D=1", "Ghost,Lion"},
		{"file data", "language=en&categories=Animals&ns%5B6%5D=1&ext_image_data=1&file_usage_data=1&file_media_types=BITMAP", "Lion.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := titles(pages(t, env, tt.query)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIntegrationCommons covers the split of Commons' links tables to their
// own cluster. The fixture serves both clusters from one database, but the
// queries take the same route as on Toolforge: base query on the links
// cluster, revision and page_props filters deferred to the core cluster.
func TestIntegrationCommons(t *testing.T) {
	env := setupFixture(t)
	tests := []struct {
		name, query, want string
	}{
		{"category", "language=commons&project=wikimedia&categories=Animals&depth=1&ns%5B0%5D=1", "Ghost,Lion,Tiger"},
		{"deferred revision filter with seed", "language=commons&project=wikimedia&categories=Animals&depth=1&after=20250601000000&ns%5B0%5D=1", "Lion"},
		{"deferred page_props filter", "language=commons&project=wikimedia&categories=Animals&depth=1&wikidata_item=without&ns%5B0%5D=1", "Ghost"},
		{"hidden categories in a second query", "language=commons&project=wikimedia&categories=Animals&depth=1&skip_hidden_categories=1&ns%5B0%5D=1", "Lion,Tiger"},
		{"links and revision", "language=commons&project=wikimedia&links_to_any=Tiger%0AMissing&edits%5Banons%5D=no&ns%5B0%5D=1", "Lion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := pages(t, env, tt.query)
			if got := titles(ps); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			for _, p := range ps {
				if p.Touched == "" {
					t.Errorf("%s has no last change timestamp", p.Title)
				}
			}
		})
	}
}

func TestIntegrationMetadata(t *testing.T) {
	env := setupFixture(t)
	ps := pages(t, env, "language=en&categories=Animals&ns%5B0%5D=1&wikidata_item=any&add_image=1&add_coordinates=1&add_defaultsort=1&add_disambiguation=1")
	if len(ps) != 1 {
		t.Fatalf("got %d pages", len(ps))
	}
	p := ps[0]
	if p.ID != 1 || p.Len != 1000 || p.Touched != "20260101000000" || p.Q != "Q1" {
		t.Errorf("page = %+v", p)
	}
	for k, want := range map[string]any{
		"image": "Lion.jpg", "coordinates": "-2.333333/34.833333", "defaultsort": "Lion, The",
		"disambiguation": false, "wikidata": "Q1",
	} {
		if got := p.Metadata[k]; got != want {
			t.Errorf("metadata[%s] = %v, want %v", k, got, want)
		}
	}

	files := pages(t, env, "language=en&categories=Animals&ns%5B6%5D=1&ext_image_data=1&file_usage_data=1")
	if len(files) != 1 || files[0].Metadata["img_size"] != float64(12345) || files[0].Metadata["fileusage"] != "dewiki:0::Löwe" {
		t.Errorf("file metadata = %+v", files)
	}

	items := pages(t, env, "language=en&categories=Animals&depth=1&common_wiki=wikidata&sortby=title&ns%5B0%5D=1")
	if titles(items) != "Q1,Q2" || items[0].Title != "Q1" {
		t.Errorf("items sorted by label = %+v", items)
	}

	redlinks := pages(t, env, "language=en&categories=Animals&show_redlinks=1&ns%5B0%5D=1")
	if len(redlinks) != 1 || redlinks[0].Metadata["redlink_count"] != float64(1) {
		t.Errorf("redlinks = %+v", redlinks)
	}
}

func TestIntegrationFormats(t *testing.T) {
	env := setupFixture(t)
	base := "language=en&categories=Animals&depth=1&ns%5B0%5D=1&add_coordinates=1&doit=1&format="
	for format, want := range map[string]string{
		"jsonl": `"title":"Lion"`,
		"csv":   "2,Lion,1,,1000,20260101000000,-2.333333/34.833333",
		"tsv":   "2\tLion\t1\t\t1000",
		"wiki":  "| [[Lion|]] || 1 || 0 || 1000",
		"kml":   "<coordinates>34.833333,-2.333333,0</coordinates>",
		"plain": "Ghost\nLion\nTiger\n",
	} {
		body := string(query(t, env, base+format+"&sortby=title").Body)
		if !strings.Contains(body, want) {
			t.Errorf("%s output lacks %q:\n%s", format, want, body)
		}
	}
	params, _ := ParseParams(base + "html")
	r := NewRun(env, params)
	if err := r.Execute(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Render(t.Context()); err == nil {
		t.Error("format=html should be rejected")
	}
}
