// Package audnexus implements the metadata.Provider interface against
// https://api.audnex.us (book + chapter lookups) and Audible's catalog API
// (search — Audnexus itself has no book search).
package audnexus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"audiobookr/internal/metadata"
	"audiobookr/internal/metadata/audible"
)

type Provider struct {
	baseURL   string
	http      *http.Client
	userAgent string
	search    *audible.Client
	limiter   *limiter
}

func New(baseURL, userAgent string) *Provider {
	return &Provider{
		baseURL:   strings.TrimRight(baseURL, "/"),
		http:      &http.Client{Timeout: 30 * time.Second},
		userAgent: userAgent,
		search:    audible.New(userAgent),
		// Audnexus allows 100 req/min per source; stay well under it.
		limiter: newLimiter(80, time.Minute),
	}
}

func (p *Provider) Search(ctx context.Context, q metadata.SearchQuery) ([]metadata.SearchResult, error) {
	return p.search.Search(ctx, q)
}

// ValidASIN reports whether s looks like an ASIN (10 alphanumeric chars).
func ValidASIN(s string) bool {
	if len(s) != 10 {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// apiBook mirrors the live /books/{asin} response shape.
type apiBook struct {
	ASIN     string `json:"asin"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Authors  []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Narrators []struct {
		Name string `json:"name"`
	} `json:"narrators"`
	Publisher string `json:"publisherName"`
	// releaseDate is RFC3339; we keep the year.
	ReleaseDate   string `json:"releaseDate"`
	RuntimeMin    int    `json:"runtimeLengthMin"`
	Language      string `json:"language"`
	Image         string `json:"image"`
	Description   string `json:"description"`
	Summary       string `json:"summary"`
	SeriesPrimary *struct {
		Name     string `json:"name"`
		Position string `json:"position"`
	} `json:"seriesPrimary"`
	Genres []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"genres"`
}

func (p *Provider) GetBook(ctx context.Context, asin, region string) (*metadata.Book, error) {
	if !ValidASIN(asin) {
		return nil, fmt.Errorf("%q is not a valid ASIN (must be 10 letters/digits)", asin)
	}
	var ab apiBook
	u := fmt.Sprintf("%s/books/%s?region=%s", p.baseURL, asin, region)
	if err := p.getJSON(ctx, u, &ab); err != nil {
		return nil, err
	}
	book := &metadata.Book{
		ASIN:        ab.ASIN,
		Region:      region,
		Title:       ab.Title,
		Subtitle:    ab.Subtitle,
		Publisher:   ab.Publisher,
		Language:    ab.Language,
		Description: ab.Description,
		// `summary` is the full blurb but arrives as HTML; `description` is a
		// ~250-char teaser ending in "...". Keep both, flattened to text.
		Summary:    htmlToText(ab.Summary),
		RuntimeMin: ab.RuntimeMin,
		CoverURL:   ab.Image,
	}
	if len(ab.ReleaseDate) >= 4 {
		book.Year = ab.ReleaseDate[:4]
	}
	if len(ab.ReleaseDate) >= 10 {
		book.ReleaseDate = ab.ReleaseDate[:10]
	}
	for _, a := range ab.Authors {
		book.Authors = append(book.Authors, a.Name)
	}
	for _, n := range ab.Narrators {
		book.Narrators = append(book.Narrators, n.Name)
	}
	if ab.SeriesPrimary != nil {
		book.SeriesName = ab.SeriesPrimary.Name
		book.SeriesPosition = ab.SeriesPrimary.Position
	}
	for _, g := range ab.Genres {
		if g.Type == "genre" {
			book.Genres = append(book.Genres, g.Name)
		}
	}
	return book, nil
}

// apiChapters mirrors the live /books/{asin}/chapters response.
type apiChapters struct {
	ASIN         string `json:"asin"`
	BrandIntroMs int64  `json:"brandIntroDurationMs"`
	BrandOutroMs int64  `json:"brandOutroDurationMs"`
	IsAccurate   bool   `json:"isAccurate"`
	RuntimeMs    int64  `json:"runtimeLengthMs"`
	Chapters     []struct {
		Title    string `json:"title"`
		StartMs  int64  `json:"startOffsetMs"`
		LengthMs int64  `json:"lengthMs"`
	} `json:"chapters"`
}

func (p *Provider) GetChapters(ctx context.Context, asin, region string) (*metadata.ChapterInfo, error) {
	if !ValidASIN(asin) {
		return nil, fmt.Errorf("%q is not a valid ASIN", asin)
	}
	var ac apiChapters
	u := fmt.Sprintf("%s/books/%s/chapters?region=%s", p.baseURL, asin, region)
	if err := p.getJSON(ctx, u, &ac); err != nil {
		return nil, err
	}
	info := &metadata.ChapterInfo{
		ASIN:         ac.ASIN,
		RuntimeMs:    ac.RuntimeMs,
		IsAccurate:   ac.IsAccurate,
		BrandIntroMs: ac.BrandIntroMs,
		BrandOutroMs: ac.BrandOutroMs,
	}
	for _, ch := range ac.Chapters {
		info.Chapters = append(info.Chapters, metadata.Chapter{
			Title: ch.Title, StartMs: ch.StartMs, LengthMs: ch.LengthMs,
		})
	}
	return info, nil
}

// getJSON performs a rate-limited GET with up to 3 retries on transient
// failures (429/408/5xx/network), honoring Retry-After and adding jitter.
// 404 maps to metadata.NotFound; other 4xx fail immediately.
func (p *Provider) getJSON(ctx context.Context, url string, out any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
			backoff += time.Duration(rand.Int64N(int64(300 * time.Millisecond)))
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := p.limiter.wait(ctx); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", p.userAgent)
		req.Header.Set("Accept", "application/json")

		resp, err := p.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		func() {
			defer resp.Body.Close()
			switch {
			case resp.StatusCode == http.StatusOK:
				lastErr = json.NewDecoder(resp.Body).Decode(out)
				if lastErr != nil {
					lastErr = fmt.Errorf("decode %s: %w", url, lastErr)
				} else {
					lastErr = errDone
				}
			case resp.StatusCode == http.StatusNotFound:
				lastErr = metadata.NotFound("not found (check the ASIN and region)")
			case resp.StatusCode == http.StatusTooManyRequests,
				resp.StatusCode == http.StatusRequestTimeout,
				resp.StatusCode >= 500:
				if ra := retryAfter(resp); ra > 0 {
					select {
					case <-time.After(ra):
					case <-ctx.Done():
					}
				}
				lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
			default:
				io.Copy(io.Discard, resp.Body)
				lastErr = permError{fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)}
			}
		}()
		if lastErr == errDone {
			return nil
		}
		if metadata.IsNotFound(lastErr) {
			return lastErr
		}
		var pe permError
		if errors.As(lastErr, &pe) {
			return pe.error
		}
	}
	return lastErr
}

var (
	htmlBreak = regexp.MustCompile(`(?i)<br\s*/?>|</p\s*>|</div\s*>|</li\s*>`)
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
	htmlSpace = regexp.MustCompile(`[ \t]{2,}`)
	htmlBlank = regexp.MustCompile(`\n{3,}`)
)

// htmlToText flattens the provider's HTML blurb into plain text suitable for
// an mp4 metadata atom: block-level tags become paragraph breaks, the
// remaining tags are dropped, and entities are decoded.
func htmlToText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = htmlBreak.ReplaceAllString(s, "\n\n")
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ") // non-breaking space
	s = htmlSpace.ReplaceAllString(s, " ")
	s = htmlBlank.ReplaceAllString(s, "\n\n")
	// Tag removal leaves stray spaces at line edges (e.g. from "</p> <p>").
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

var errDone = errors.New("done")

type permError struct{ error }

func retryAfter(resp *http.Response) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 && secs <= 120 {
			return time.Duration(secs) * time.Second
		}
	}
	return 0
}

// limiter is a minimal token bucket: n tokens refilled every period.
type limiter struct {
	tokens chan struct{}
}

func newLimiter(n int, period time.Duration) *limiter {
	l := &limiter{tokens: make(chan struct{}, n)}
	for i := 0; i < n; i++ {
		l.tokens <- struct{}{}
	}
	go func() {
		tick := time.NewTicker(period / time.Duration(n))
		defer tick.Stop()
		for range tick.C {
			select {
			case l.tokens <- struct{}{}:
			default:
			}
		}
	}()
	return l
}

func (l *limiter) wait(ctx context.Context) error {
	select {
	case <-l.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
