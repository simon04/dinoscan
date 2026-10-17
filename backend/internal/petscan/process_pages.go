package petscan

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"strings"

	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

// pageField is an optional column of addPageProperties.
type pageField struct {
	// table is the table the column reads besides page.
	table  string
	column replica.Fragment
	// apply stores the scanned value.
	apply func(e *Entry, v sql.NullString)
}

// addPageProperties adds page image, coordinates, default sort key etc.,
// as requested by the output options and the sort order.
func (r *Run) addPageProperties(ctx context.Context, pl *PageList) error {
	p := r.params
	isKML := p.Get("format") == "kml"
	sortBy := p.Get("sortby")
	var fields []pageField
	if p.Has("add_image") || isKML {
		fields = append(fields, pageField{"page_props",
			"(SELECT pp_value FROM page_props WHERE pp_page=page_id AND pp_propname IN ('page_image','page_image_free') LIMIT 1)",
			func(e *Entry, v sql.NullString) { e.Image = v.String }})
	}
	if p.Has("add_coordinates") || isKML {
		fields = append(fields, pageField{"geo_tags",
			"(SELECT CONCAT(gt_lat,',',gt_lon) FROM geo_tags WHERE gt_primary=1 AND gt_globe='earth' AND gt_page_id=page_id LIMIT 1)",
			func(e *Entry, v sql.NullString) { e.Coordinates = parseCoordinates(v.String) }})
	}
	if p.Has("add_defaultsort") || sortBy == "defaultsort" {
		fields = append(fields, pageField{"page_props",
			"(SELECT pp_value FROM page_props WHERE pp_page=page_id AND pp_propname='defaultsort' LIMIT 1)",
			func(e *Entry, v sql.NullString) { e.DefaultSort = v.String }})
	}
	if p.Has("add_disambiguation") {
		fields = append(fields, pageField{"page_props",
			"(SELECT 1 FROM page_props WHERE pp_page=page_id AND pp_propname='disambiguation' LIMIT 1)",
			func(e *Entry, v sql.NullString) {
				e.Disambiguation = no
				if v.Valid {
					e.Disambiguation = yes
				}
			}})
	}
	if sortBy == "incoming_links" {
		fields = append(fields, pageField{"pagelinks",
			"(SELECT count(*) FROM pagelinks,linktarget WHERE pl_target_id=lt_id AND lt_namespace=page_namespace AND lt_title=page_title AND pl_from_namespace=0)",
			func(e *Entry, v sql.NullString) { e.IncomingLinks = parseCount(v) }})
	}
	if sortBy == "sitelinks" && !pl.HasSitelinkCounts {
		if pl.IsWikidata() {
			fields = append(fields, pageField{"wb_items_per_site",
				"(SELECT count(*) FROM wb_items_per_site WHERE page_namespace=0 AND ips_item_id=SUBSTRING(page_title,2))",
				func(e *Entry, v sql.NullString) { e.SitelinkCount = parseCount(v) }})
		} else {
			fields = append(fields, pageField{"langlinks",
				"(SELECT count(*) FROM langlinks WHERE ll_from=page_id)",
				func(e *Entry, v sql.NullString) { e.SitelinkCount = parseCount(v) }})
		}
	}
	if len(fields) == 0 || pl.Len() == 0 {
		return nil
	}

	// The tables are on different hosts on Commons: one query per cluster.
	byCluster := map[replica.Cluster][]pageField{}
	for _, f := range fields {
		cluster, err := replica.ClusterFor(pl.Wiki, "page", f.table)
		if err != nil {
			return err
		}
		byCluster[cluster] = append(byCluster[cluster], f)
	}
	for cluster, fields := range byCluster {
		var queries []*replica.Query
		for _, b := range pl.batches(pageBatchSize) {
			q := replica.New("SELECT page_title,page_namespace")
			for _, f := range fields {
				q.Add(",").Add(f.column)
			}
			queries = append(queries, q.Add(" FROM page WHERE ").Append(b.where()))
		}
		err := r.queryAll(ctx, pl.Wiki, cluster, queries, func(rows *sql.Rows) error {
			var title string
			var ns int
			values := make([]sql.NullString, len(fields))
			dest := []any{&title, &ns}
			for i := range values {
				dest = append(dest, &values[i])
			}
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			if e := pl.Get(wiki.NewTitle(title, ns)); e != nil {
				for i, f := range fields {
					f.apply(e, values[i])
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func parseCount(v sql.NullString) *int {
	n, err := strconv.Atoi(v.String)
	if !v.Valid || err != nil {
		return nil
	}
	return &n
}

// parseCoordinates parses "lat,lon".
func parseCoordinates(s string) *Coordinates {
	lat, lon, ok := strings.Cut(s, ",")
	if !ok {
		return nil
	}
	la, err1 := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	lo, err2 := strconv.ParseFloat(strings.TrimSpace(lon), 64)
	if err1 != nil || err2 != nil {
		return nil
	}
	return &Coordinates{Lat: la, Lon: lo}
}

// addFileData adds file usage and file metadata, and filters on the media
// type.
func (r *Run) addFileData(ctx context.Context, pl *PageList) error {
	p := r.params
	mediaTypes := mediaTypesFilter(p.Get("file_media_types"))
	sortBy := p.Get("sortby")
	if p.Has("giu") || p.Has("file_usage_data") {
		if err := r.addFileUsage(ctx, pl, p.Has("file_usage_data_ns0")); err != nil {
			return err
		}
	}
	if p.Has("ext_image_data") || sortBy == "filesize" || sortBy == "uploaddate" || len(mediaTypes) > 0 {
		if err := r.addFileMetadata(ctx, pl, mediaTypes); err != nil {
			return err
		}
	}
	if len(mediaTypes) > 0 {
		pl.Retain(func(e *Entry) bool { return hasFileData(e) && slices.Contains(mediaTypes, e.File.MediaType) })
	}
	return nil
}

// mediaTypesFilter parses file_media_types, e.g. "BITMAP|VIDEO".
func mediaTypesFilter(s string) []string {
	var ret []string
	for _, t := range strings.Split(s, "|") {
		if t = strings.ToUpper(strings.TrimSpace(t)); t != "" {
			ret = append(ret, t)
		}
	}
	return ret
}

func fileInfo(e *Entry) *FileInfo {
	if e.File == nil {
		e.File = &FileInfo{}
	}
	return e.File
}

// addFileUsage adds the global usage of the files, from globalimagelinks.
func (r *Run) addFileUsage(ctx context.Context, pl *PageList, ns0Only bool) error {
	var queries []*replica.Query
	for _, b := range pl.batchesInNamespace(pageBatchSize, 6) {
		q := replica.New("SELECT gil_to,gil_wiki,gil_page_namespace_id,gil_page,gil_page_namespace,gil_page_title " +
			"FROM globalimagelinks WHERE gil_to").In(replica.Strings(b.Titles))
		if ns0Only {
			q.Add(" AND gil_page_namespace_id=0")
		}
		queries = append(queries, q)
	}
	cluster, err := replica.ClusterFor(pl.Wiki, "globalimagelinks")
	if err != nil {
		return err
	}
	return r.queryAll(ctx, pl.Wiki, cluster, queries, func(rows *sql.Rows) error {
		var file, usageWiki, nsName, title string
		var ns int
		var pageID int64
		if err := rows.Scan(&file, &usageWiki, &ns, &pageID, &nsName, &title); err != nil {
			return err
		}
		if e := pl.Get(wiki.NewTitle(file, 6)); e != nil {
			fi := fileInfo(e)
			fi.Usage = append(fi.Usage, FileUsage{
				Wiki:          usageWiki,
				Title:         wiki.NewTitle(title, ns),
				NamespaceName: nsName,
				PageID:        pageID,
			})
		}
		return nil
	})
}

// addFileMetadata adds size, dimensions, media type etc. of the files,
// optionally only of the given media types.
func (r *Run) addFileMetadata(ctx context.Context, pl *PageList, mediaTypes []string) error {
	var queries []*replica.Query
	for _, b := range pl.batchesInNamespace(pageBatchSize, 6) {
		var q *replica.Query
		if r.env.UseFileTable {
			q = replica.New("SELECT file_name,fr_size,fr_width,fr_height,ft_media_type,ft_major_mime,ft_minor_mime," +
				"actor_name,fr_timestamp,fr_sha1 FROM file,filerevision,filetypes,actor " +
				"WHERE file_latest=fr_id AND fr_actor=actor_id AND file_type=ft_id AND fr_deleted=0 AND file_name").
				In(replica.Strings(b.Titles))
			if len(mediaTypes) > 0 {
				q.Add(" AND ft_media_type").In(replica.Strings(mediaTypes))
			}
		} else {
			q = replica.New("SELECT img_name,img_size,img_width,img_height,img_media_type,img_major_mime,img_minor_mime," +
				"img_user_text,img_timestamp,img_sha1 FROM image_compat WHERE img_name").In(replica.Strings(b.Titles))
			if len(mediaTypes) > 0 {
				q.Add(" AND img_media_type").In(replica.Strings(mediaTypes))
			}
		}
		queries = append(queries, q)
	}
	return r.queryAll(ctx, pl.Wiki, replica.Core, queries, func(rows *sql.Rows) error {
		var name string
		var f FileInfo
		var user sql.NullString
		if err := rows.Scan(&name, &f.Size, &f.Width, &f.Height, &f.MediaType, &f.MajorMIME, &f.MinorMIME, &user, &f.Timestamp, &f.SHA1); err != nil {
			return err
		}
		if e := pl.Get(wiki.NewTitle(name, 6)); e != nil {
			fi := fileInfo(e)
			f.Usage = fi.Usage
			f.User = user.String
			f.HasData = true
			*fi = f
		}
		return nil
	})
}
