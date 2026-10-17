package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UserAgent identifies the tool to Wikimedia servers, as their policy asks.
const UserAgent = "DinoScan (https://dinoscan.toolforge.org/; https://github.com/simon04/dinoscan)"

// cacheTTL is how long site matrix and siteinfo responses are reused.
const cacheTTL = 24 * time.Hour

// ErrUnknownWiki is returned for a wiki not in the site matrix.
var ErrUnknownWiki = errors.New("unknown wiki")

// Client is a MediaWiki API client that caches the site matrix and the
// siteinfo of each wiki. It is safe for concurrent use.
type Client struct {
	HTTP *http.Client
	// SiteMatrixURL is the API serving action=sitematrix.
	SiteMatrixURL string

	mu         sync.Mutex
	matrix     *SiteMatrix
	matrixTime time.Time
	siteInfo   map[string]cachedSiteInfo
}

type cachedSiteInfo struct {
	info    *SiteInfo
	fetched time.Time
}

// NewClient returns a client with sensible timeouts.
func NewClient() *Client {
	return &Client{
		HTTP:          &http.Client{Timeout: 2 * time.Minute},
		SiteMatrixURL: "https://meta.wikimedia.org/w/api.php",
		siteInfo:      map[string]cachedSiteInfo{},
	}
}

// Do sends a request with the tool's User-Agent and decodes a JSON response
// into out. Responses other than 2xx are errors.
func (c *Client) Do(req *http.Request, out any) error {
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("%s returned HTTP %d: %s", req.URL.Host, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("invalid JSON from %s: %w", req.URL.Host, err)
	}
	return nil
}

// apiError is the error member of a MediaWiki API response.
type apiError struct {
	Code string `json:"code"`
	Info string `json:"info"`
}

// api posts params to the action API at apiURL, in JSON format version 2,
// and decodes the response into out; callers check its error member.
func (c *Client) api(ctx context.Context, apiURL string, params url.Values, out any) error {
	params.Set("format", "json")
	params.Set("formatversion", "2")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.Do(req, out)
}

// SiteMatrix returns the cached site matrix of all Wikimedia wikis.
func (c *Client) SiteMatrix(ctx context.Context) (*SiteMatrix, error) {
	c.mu.Lock()
	if c.matrix != nil && time.Since(c.matrixTime) < cacheTTL {
		defer c.mu.Unlock()
		return c.matrix, nil
	}
	c.mu.Unlock()

	var resp struct {
		SiteMatrix map[string]json.RawMessage `json:"sitematrix"`
		Error      *apiError                  `json:"error"`
	}
	params := url.Values{"action": {"sitematrix"}}
	if err := c.api(ctx, c.SiteMatrixURL, params, &resp); err != nil {
		return nil, fmt.Errorf("cannot load site matrix: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("cannot load site matrix: %s", resp.Error.Info)
	}
	matrix, err := parseSiteMatrix(resp.SiteMatrix)
	if err != nil {
		return nil, fmt.Errorf("cannot parse site matrix: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.matrix, c.matrixTime = matrix, time.Now()
	return matrix, nil
}

// APIURL returns the action API endpoint of wiki.
func (c *Client) APIURL(ctx context.Context, wiki string) (string, error) {
	matrix, err := c.SiteMatrix(ctx)
	if err != nil {
		return "", err
	}
	server, ok := matrix.ServerForWiki(wiki)
	if !ok {
		return "", fmt.Errorf("%w %q", ErrUnknownWiki, wiki)
	}
	return server + "/w/api.php", nil
}

// SiteInfo returns the cached server and namespaces of wiki.
func (c *Client) SiteInfo(ctx context.Context, wiki string) (*SiteInfo, error) {
	c.mu.Lock()
	if cached, ok := c.siteInfo[wiki]; ok && time.Since(cached.fetched) < cacheTTL {
		defer c.mu.Unlock()
		return cached.info, nil
	}
	c.mu.Unlock()

	matrix, err := c.SiteMatrix(ctx)
	if err != nil {
		return nil, err
	}
	server, ok := matrix.ServerForWiki(wiki)
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownWiki, wiki)
	}
	var resp struct {
		Query struct {
			Namespaces       map[string]Namespace `json:"namespaces"`
			NamespaceAliases []struct {
				ID    int    `json:"id"`
				Alias string `json:"alias"`
			} `json:"namespacealiases"`
		} `json:"query"`
		Error *apiError `json:"error"`
	}
	params := url.Values{
		"action": {"query"},
		"meta":   {"siteinfo"},
		"siprop": {"namespaces|namespacealiases"},
	}
	if err := c.api(ctx, server+"/w/api.php", params, &resp); err != nil {
		return nil, fmt.Errorf("cannot load siteinfo of %s: %w", wiki, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("cannot load siteinfo of %s: %s", wiki, resp.Error.Info)
	}
	namespaces := make([]Namespace, 0, len(resp.Query.Namespaces))
	for _, ns := range resp.Query.Namespaces {
		namespaces = append(namespaces, ns)
	}
	aliases := map[string]int{}
	for _, a := range resp.Query.NamespaceAliases {
		aliases[a.Alias] = a.ID
	}
	info := NewSiteInfo(wiki, server, namespaces, aliases)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.siteInfo[wiki] = cachedSiteInfo{info, time.Now()}
	return info, nil
}

// SearchResult is one hit of list=search.
type SearchResult struct {
	NS     int    `json:"ns"`
	Title  string `json:"title"`
	PageID int64  `json:"pageid"`
}

// Search runs a full-text search on wiki and returns up to max hits.
// namespaces is a list=search srnamespace value, e.g. "0|14" or "*".
func (c *Client) Search(ctx context.Context, wiki, query, namespaces string, max int) ([]SearchResult, error) {
	apiURL, err := c.APIURL(ctx, wiki)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	offset := 0
	for len(results) < max {
		var resp struct {
			Query struct {
				Search []SearchResult `json:"search"`
			} `json:"query"`
			Continue *struct {
				SrOffset int `json:"sroffset"`
			} `json:"continue"`
			Error *apiError `json:"error"`
		}
		params := url.Values{
			"action":      {"query"},
			"list":        {"search"},
			"srsearch":    {query},
			"srnamespace": {namespaces},
			"srlimit":     {strconv.Itoa(min(max-len(results), 500))},
			"srprop":      {""},
			"sroffset":    {strconv.Itoa(offset)},
		}
		if err := c.api(ctx, apiURL, params, &resp); err != nil {
			return nil, fmt.Errorf("search on %s: %w", wiki, err)
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("search on %s: %s", wiki, resp.Error.Info)
		}
		results = append(results, resp.Query.Search...)
		if resp.Continue == nil || len(resp.Query.Search) == 0 {
			break
		}
		offset = resp.Continue.SrOffset
	}
	if len(results) > max {
		results = results[:max]
	}
	return results, nil
}
