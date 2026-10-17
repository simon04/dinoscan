// Package replica connects to the read-only Wiki Replicas on Toolforge and
// builds the SQL sent to them.
//
// See https://wikitech.wikimedia.org/wiki/Help:Wiki_Replicas
package replica

import (
	"fmt"
	"slices"
	"strings"
)

// Cluster is a replica database cluster holding some or all of a wiki's
// tables.
//
// Most wikis keep everything in their core database. A few have tables split
// off into an extension database on its own host, addressed by prefixing the
// replica hostname with the extension name:
//
//   - links.commonswiki holds Commons' links tables (since September 2026);
//     page and redirect are on both hosts.
//   - termstore.wikidatawiki holds Wikidata's wbt_* term-store tables.
type Cluster int

const (
	// Core is a wiki's core database, holding every table not split off.
	Core Cluster = iota
	// Links is the Commons links-tables extension database.
	Links
	// TermStore is the Wikidata term-store extension database.
	TermStore
)

// allClusters lists every cluster, core first: a table available on more than
// one cluster is read from the earliest.
var allClusters = []Cluster{Core, Links, TermStore}

// linksTables are the tables moved to the Commons links cluster.
var linksTables = []string{
	"categorylinks",
	"collation",
	"existencelinks",
	"externallinks",
	"globalimagelinks",
	"imagelinks",
	"iwlinks",
	"langlinks",
	"linktarget",
	"pagelinks",
	"templatelinks",
}

// linksSharedTables are kept on both the core and the links cluster.
var linksSharedTables = []string{"page", "redirect"}

func (c Cluster) String() string {
	switch c {
	case Links:
		return "links"
	case TermStore:
		return "termstore"
	default:
		return "core"
	}
}

// hostPrefix is the hostname prefix identifying the cluster.
func (c Cluster) hostPrefix() string {
	switch c {
	case Links:
		return "links."
	case TermStore:
		return "termstore."
	default:
		return ""
	}
}

// serverGroup is the replica service group serving the cluster. Everything
// uses "web", whose short query timeout suits a user waiting for a result;
// the term store is read from "analytics", whose longer timeout the label
// queries need.
func (c Cluster) serverGroup() string {
	if c == TermStore {
		return "analytics"
	}
	return "web"
}

// AppliesTo reports whether wiki has this cluster at all.
func (c Cluster) AppliesTo(wiki string) bool {
	wiki = FixWikiName(wiki)
	switch c {
	case Links:
		return wiki == "commonswiki" || wiki == "testcommonswiki"
	case TermStore:
		return wiki == "wikidatawiki"
	default:
		return true
	}
}

// ownsExclusively reports whether table lives only on this cluster, and so is
// no longer readable on the core cluster of a wiki that has the split.
func (c Cluster) ownsExclusively(table string) bool {
	table = strings.ToLower(table)
	switch c {
	case Links:
		return slices.Contains(linksTables, table)
	case TermStore:
		return strings.HasPrefix(table, "wbt_")
	default:
		return false
	}
}

// Hosts reports whether a query against this cluster can read all of tables
// for wiki.
func (c Cluster) Hosts(wiki string, tables ...string) bool {
	for _, table := range tables {
		if !c.hosts(wiki, table) {
			return false
		}
	}
	return true
}

func (c Cluster) hosts(wiki, table string) bool {
	switch c {
	case Core:
		for _, ext := range []Cluster{Links, TermStore} {
			if ext.AppliesTo(wiki) && ext.ownsExclusively(table) {
				return false
			}
		}
		return true
	case Links:
		return c.ownsExclusively(table) || slices.Contains(linksSharedTables, strings.ToLower(table))
	default:
		return c.ownsExclusively(table)
	}
}

// ClusterFor returns the cluster able to serve a query reading all of tables
// on wiki. It fails when no single cluster holds them all; such a join has to
// be done in code instead.
func ClusterFor(wiki string, tables ...string) (Cluster, error) {
	for _, c := range allClusters {
		if c.AppliesTo(wiki) && c.Hosts(wiki, tables...) {
			return c, nil
		}
	}
	return Core, fmt.Errorf("tables %v of %s span several database clusters", tables, wiki)
}

// effective returns c where wiki has it, Core otherwise.
func (c Cluster) effective(wiki string) Cluster {
	if c.AppliesTo(wiki) {
		return c
	}
	return Core
}

// FixWikiName normalizes a wiki database name, e.g. be-taraskwiki to
// be_x_oldwiki.
func FixWikiName(wiki string) string {
	switch wiki {
	case "be-taraskwiki", "be-x-oldwiki", "be_taraskwiki", "be_x_oldwiki":
		return "be_x_oldwiki"
	}
	return strings.ReplaceAll(wiki, "-", "_")
}
