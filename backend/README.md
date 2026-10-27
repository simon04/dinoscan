# DinoScan backend

A re-implementation of the [PetScan](https://github.com/magnusmanske/petscan_rs)
query backend in Go. It serves

- `/api` – a PetScan-compatible query API (GET query string or POST form,
  same parameters as PetScan), reading the read-only
  [Wiki Replicas](https://wikitech.wikimedia.org/wiki/Help:Wiki_Replicas),
- `/healthz` – a health check,
- `/` – the built DinoScan frontend from `DIST_DIR`, with `index.html` as
  fallback for all other paths.

Dependencies: the Go standard library and
[go-sql-driver/mysql](https://github.com/go-sql-driver/mysql). Maintainability
is preferred over the last bit of performance.

## Layout

| Package            | Contents                                                                                  |
| ------------------ | ----------------------------------------------------------------------------------------- |
| `main.go`          | configuration from the environment, HTTP server                                           |
| `internal/server`  | HTTP handlers, admission control, error responses, static files                           |
| `internal/petscan` | parameters, data sources, combination, filters, post-processing, output formats           |
| `internal/replica` | replica hosts and clusters (Commons links split, Wikidata term store), pools, SQL builder |
| `internal/wiki`    | MediaWiki API client: site matrix, siteinfo namespaces, titles, search                    |

A query runs as in PetScan: data sources (`source_database.go`, `sources.go`)
yield page lists, which are combined (`combination.go`), then filtered and
annotated (`process*.go`), sorted and rendered (`render.go`).

## SQL

All SQL is built with `replica.Query`. Its text only accepts
`replica.Fragment`, to which only string _constants_ convert implicitly, so
every value — titles, IDs, namespaces, limits — goes through a `?` placeholder
of a server-side prepared statement. `Add` panics if placeholders and values
don't match. Titles and IDs are sent in batches of at most 15 000 (category
names 25 000), below MariaDB's limit of 65 535 placeholders.

Commons keeps its links tables (`categorylinks`, `pagelinks`, `templatelinks`,
`linktarget`, …) on a separate host since September 2026. Queries name the
tables they read and `replica.ClusterFor` routes them; filter clauses the base
query's host cannot serve are applied in a second pass on the other host
(`filters.go`).

## Configuration

| Variable                                     | Default                           | Meaning                                                                                                   |
| -------------------------------------------- | --------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `PORT`                                       | `8000`                            | HTTP port                                                                                                 |
| `DIST_DIR`                                   | `dist`                            | built frontend                                                                                            |
| `PUBLIC_URL`                                 | `https://dinoscan.toolforge.org/` | base URL for links in the output                                                                          |
| `TOOL_REPLICA_USER`, `TOOL_REPLICA_PASSWORD` | `~/replica.my.cnf`                | replica credentials; set by Toolforge                                                                     |
| `REPLICA_HOST`                               | Toolforge host names              | e.g. `127.0.0.1` for SSH tunnels                                                                          |
| `REPLICA_PORT`                               | `3306`                            | port with `REPLICA_HOST`                                                                                  |
| `REPLICA_PORTS`                              |                                   | per-host ports with `REPLICA_HOST`, e.g. `enwiki=3306,links.commonswiki=3315,termstore.wikidatawiki=3317` |
| `MAX_RESULTS`                                | `1000000`                         | most pages one query may load                                                                             |
| `USE_FILE_TABLE`                             |                                   | read file metadata from `file`/`filerevision` instead of `image_compat`                                   |
| `LOG_LEVEL`                                  | `info`                            | `debug` logs every database query                                                                         |

## Development

```sh
# tunnel to the replicas you need (keys of REPLICA_PORTS are the leading labels of the host)
ssh login.toolforge.org -N \
  -L 3306:enwiki.web.db.svc.wikimedia.cloud:3306 \
  -L 3307:wikidatawiki.web.db.svc.wikimedia.cloud:3306
REPLICA_HOST=127.0.0.1 REPLICA_PORTS=enwiki=3306,wikidatawiki=3307 \
  TOOL_REPLICA_USER=… TOOL_REPLICA_PASSWORD=… DIST_DIR=../dist go run .

# frontend against the local backend
VITE_PETSCAN_API=http://localhost:8000/api pnpm dev

curl 'http://localhost:8000/api?language=en&project=wikipedia&categories=Physics&depth=1&ns[0]=1&format=json'
```

Tests: `go test ./...` runs the unit tests; `scripts/test-mariadb.sh` also runs
the integration tests, which execute every kind of generated query against a
throwaway local MariaDB server with a tiny replica-like fixture
(`internal/petscan/integration_test.go`, needs `mariadbd`). CI runs both
(`.github/workflows/go-ci.yml`).

## Differences from PetScan

Scope is what DinoScan needs:

- Formats `json`, `jsonl`, `csv`, `tsv`, `wiki`, `kml`, `plain`, `pagepile`.
  No `html` (DinoScan is the UI), no WD-FIST (`wdf_main`), no AutoList creator
  mode.
- No PSIDs: the replicas are read-only and every query is fully described by
  its URL.

Deliberate changes, mostly fixes:

- `ns[N]` accepts `true` as well as `1` (DinoScan sends `true`, which PetScan
  ignores).
- `created_by` matches user names with spaces; label filters for properties
  and `labels_any` without languages work; `outlinks_*`/`links_to_*` across
  several namespaces are one subquery with OR'ed namespace groups instead of
  a mis-parenthesized `OR` of subqueries.
- An invalid `rxp_filter` is an error instead of being ignored; it is matched
  as `^(?:…)$`.
- Wikidata items of pages come from the wiki's own `page_props` instead of a
  title lookup on Wikidata; file usage is read row by row instead of with
  `GROUP_CONCAT`.
- Ties in sorting are broken by namespace and title, so output order is stable.
- JSON always has a `metadata` object; redlink counts appear in it.
- PagePile output redirects (HTTP 303) to the new pile.
- Errors have HTTP status 400 (bad query), 502 (database or remote service),
  503 (overloaded), 504 (timeout) or 500.

## Deployment

The Toolforge component (`toolforge.yaml`, `Procfile` in the repository root)
runs this binary, serving the frontend and the API. `go.mod` is at the
repository root because the
[Heroku Go buildpack](https://github.com/heroku/buildpacks-go) requires it
there; its `// +heroku install ./backend` directive builds only this package,
and `ignore` keeps `go` commands out of `node_modules` and `dist`. Toolforge
provides `TOOL_REPLICA_USER` and `TOOL_REPLICA_PASSWORD`.

The frontend queries `https://dinoscan.toolforge.org/api` (also from GitHub
Pages and the dev server, CORS allows it); `VITE_PETSCAN_API` overrides that.
