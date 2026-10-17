package petscan

import (
	"cmp"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"

	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// Entry is one page of a result, with whatever data has been gathered on it.
// Zero values mean "unknown"; counts that may legitimately be zero are
// pointers.
type Entry struct {
	Title   wiki.Title
	PageID  int64
	Len     int64
	Touched string // timestamp of the latest revision, YYYYMMDDHHMMSS

	WikidataItem        string // e.g. "Q42"
	WikidataLabel       string
	WikidataDescription string

	Image          string // page image file name, without namespace
	Coordinates    *Coordinates
	DefaultSort    string
	Disambiguation triState

	LinkCount     *int // outgoing links, for minlinks/maxlinks
	IncomingLinks *int
	RedlinkCount  *int
	SitelinkCount *int

	File *FileInfo
}

// Coordinates are a page's primary coordinates on Earth.
type Coordinates struct {
	Lat, Lon float64
}

// triState is a yes/no flag that may be unknown.
type triState int8

const (
	unknown triState = iota
	yes
	no
)

// json returns true, false or nil.
func (t triState) json() any {
	switch t {
	case yes:
		return true
	case no:
		return false
	default:
		return nil
	}
}

// FileInfo is data on a file page.
type FileInfo struct {
	Usage   []FileUsage
	HasData bool // the img_* fields below are set
	Size    int64
	Width   int64
	Height  int64
	// MediaType is e.g. BITMAP or VIDEO.
	MediaType string
	MajorMIME string
	MinorMIME string
	User      string
	Timestamp string
	SHA1      string
}

// FileUsage is one use of a file, from Commons' globalimagelinks.
type FileUsage struct {
	Wiki          string
	Title         wiki.Title
	NamespaceName string
	PageID        int64
}

// fileDataKeys are the column names of the FileInfo fields, as in PetScan's
// output.
var fileDataKeys = []string{
	"img_size",
	"img_width",
	"img_height",
	"img_media_type",
	"img_major_mime",
	"img_minor_mime",
	"img_user_text",
	"img_timestamp",
	"img_sha1",
}

// field returns the FileInfo field for one of fileDataKeys, as a JSON value.
func (f *FileInfo) field(key string) (any, bool) {
	if f == nil || !f.HasData {
		return nil, false
	}
	switch key {
	case "img_size":
		return f.Size, true
	case "img_width":
		return f.Width, true
	case "img_height":
		return f.Height, true
	case "img_media_type":
		return f.MediaType, true
	case "img_major_mime":
		return f.MajorMIME, true
	case "img_minor_mime":
		return f.MinorMIME, true
	case "img_user_text":
		return f.User, true
	case "img_timestamp":
		return f.Timestamp, true
	case "img_sha1":
		return f.SHA1, true
	}
	return nil, false
}

// usageString joins the file usage as wiki:ns:nsname:title|….
func (f *FileInfo) usageString(underscores bool) string {
	if f == nil {
		return ""
	}
	parts := make([]string, len(f.Usage))
	for i, u := range f.Usage {
		title := u.Title.Name
		if underscores {
			title = u.Title.Underscored()
		}
		parts[i] = u.Wiki + ":" + strconv.Itoa(u.Title.NS) + ":" + u.NamespaceName + ":" + title
	}
	return strings.Join(parts, "|")
}

// sortEntries sorts by the PetScan sortby parameter, or shuffles for
// "random". Ties are broken by namespace and title, so the order is
// deterministic.
func sortEntries(entries []*Entry, sortBy string, descending, isWikidata bool) {
	if sortBy == "random" {
		rand.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })
		return
	}
	key := sortKey(sortBy, isWikidata)
	slices.SortStableFunc(entries, func(a, b *Entry) int {
		c := key(a, b)
		if descending {
			c = -c
		}
		if c != 0 {
			return c
		}
		return cmp.Or(cmp.Compare(a.Title.NS, b.Title.NS), strings.Compare(a.Title.Name, b.Title.Name))
	})
}

// sortKey returns the comparison for sortBy. Unknown values sort after known
// ones (before them when descending).
func sortKey(sortBy string, isWikidata bool) func(a, b *Entry) int {
	switch sortBy {
	case "title":
		if isWikidata {
			return func(a, b *Entry) int {
				return strings.Compare(strings.ToLower(labelOrTitle(a)), strings.ToLower(labelOrTitle(b)))
			}
		}
		return func(a, b *Entry) int { return strings.Compare(a.Title.Name, b.Title.Name) }
	case "ns_title":
		return func(a, b *Entry) int {
			return cmp.Or(cmp.Compare(a.Title.NS, b.Title.NS), strings.Compare(a.Title.Name, b.Title.Name))
		}
	case "size":
		return func(a, b *Entry) int { return compareKnown(a.Len, b.Len, a.PageID != 0, b.PageID != 0) }
	case "date":
		return func(a, b *Entry) int { return compareString(a.Touched, b.Touched) }
	case "redlinks":
		return func(a, b *Entry) int { return comparePtr(a.RedlinkCount, b.RedlinkCount) }
	case "incoming_links":
		return func(a, b *Entry) int { return comparePtr(a.IncomingLinks, b.IncomingLinks) }
	case "sitelinks":
		return func(a, b *Entry) int { return comparePtr(a.SitelinkCount, b.SitelinkCount) }
	case "defaultsort":
		return func(a, b *Entry) int {
			return compareString(defaultSortKey(a, isWikidata), defaultSortKey(b, isWikidata))
		}
	case "filesize":
		return func(a, b *Entry) int {
			return compareKnown(fileSize(a), fileSize(b), hasFileData(a), hasFileData(b))
		}
	case "uploaddate":
		return func(a, b *Entry) int { return compareString(fileTimestamp(a), fileTimestamp(b)) }
	default:
		return func(a, b *Entry) int { return compareKnown(a.PageID, b.PageID, a.PageID != 0, b.PageID != 0) }
	}
}

func labelOrTitle(e *Entry) string {
	return cmp.Or(e.WikidataLabel, e.Title.Name)
}

func defaultSortKey(e *Entry, isWikidata bool) string {
	if e.DefaultSort != "" {
		return e.DefaultSort
	}
	if isWikidata {
		return e.WikidataLabel
	}
	return e.Title.Name
}

func hasFileData(e *Entry) bool { return e.File != nil && e.File.HasData }

func fileSize(e *Entry) int64 {
	if hasFileData(e) {
		return e.File.Size
	}
	return 0
}

func fileTimestamp(e *Entry) string {
	if hasFileData(e) {
		return e.File.Timestamp
	}
	return ""
}

func compareKnown[T cmp.Ordered](a, b T, aKnown, bKnown bool) int {
	switch {
	case aKnown && bKnown:
		return cmp.Compare(a, b)
	case aKnown:
		return -1
	case bKnown:
		return 1
	}
	return 0
}

func compareString(a, b string) int {
	return compareKnown(a, b, a != "", b != "")
}

func comparePtr(a, b *int) int {
	var av, bv int
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return compareKnown(av, bv, a != nil, b != nil)
}
