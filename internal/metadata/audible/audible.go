// Package audible searches Audible's public catalog API for ASIN candidates.
// The API needs no auth; this is the same route audiobookshelf and the Plex
// Audnexus agent use. It is unofficial, so failures here must degrade to
// manual ASIN entry, never block the app.
package audible

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"audioborker/internal/metadata"
)

// regionTLD maps Audnexus/Audible region codes to catalog API TLDs.
var regionTLD = map[string]string{
	"us": ".com", "ca": ".ca", "uk": ".co.uk", "au": ".com.au", "fr": ".fr",
	"de": ".de", "jp": ".co.jp", "it": ".it", "in": ".in", "es": ".es",
}

type Client struct {
	http      *http.Client
	userAgent string
}

func New(userAgent string) *Client {
	return &Client{
		http:      &http.Client{Timeout: 15 * time.Second},
		userAgent: userAgent,
	}
}

// product is the subset of the catalog response we consume.
type product struct {
	ASIN     string `json:"asin"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Authors  []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Narrators []struct {
		Name string `json:"name"`
	} `json:"narrators"`
	ReleaseDate   string            `json:"release_date"`
	RuntimeMin    int               `json:"runtime_length_min"`
	Language      string            `json:"language"`
	ProductImages map[string]string `json:"product_images"`
}

// Search queries the catalog. Empty query fields are omitted.
func (c *Client) Search(ctx context.Context, q metadata.SearchQuery) ([]metadata.SearchResult, error) {
	tld, ok := regionTLD[q.Region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q", q.Region)
	}
	params := url.Values{
		"num_results":      {"25"},
		"products_sort_by": {"Relevance"},
		"response_groups":  {"contributors,product_desc,media,product_attrs"},
		"image_sizes":      {"500"},
	}
	if q.Keywords != "" {
		params.Set("keywords", q.Keywords)
	}
	if q.Title != "" {
		params.Set("title", q.Title)
	}
	if q.Author != "" {
		params.Set("author", q.Author)
	}

	u := fmt.Sprintf("https://api.audible%s/1.0/catalog/products?%s", tld, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("audible search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audible search: HTTP %d", resp.StatusCode)
	}

	var body struct {
		Products []product `json:"products"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("audible search: decode: %w", err)
	}

	out := make([]metadata.SearchResult, 0, len(body.Products))
	for _, p := range body.Products {
		if p.ASIN == "" || p.Title == "" {
			continue
		}
		r := metadata.SearchResult{
			ASIN:       p.ASIN,
			Region:     q.Region,
			Title:      p.Title,
			Subtitle:   p.Subtitle,
			Year:       yearOf(p.ReleaseDate),
			Language:   p.Language,
			RuntimeMin: p.RuntimeMin,
			CoverURL:   p.ProductImages["500"],
		}
		for _, a := range p.Authors {
			r.Authors = append(r.Authors, a.Name)
		}
		for _, n := range p.Narrators {
			r.Narrators = append(r.Narrators, n.Name)
		}
		out = append(out, r)
	}
	return out, nil
}

func yearOf(date string) string {
	if len(date) >= 4 {
		return strings.SplitN(date, "-", 2)[0]
	}
	return ""
}
