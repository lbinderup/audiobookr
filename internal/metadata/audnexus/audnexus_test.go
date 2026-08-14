package audnexus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"audioborker/internal/metadata"
)

// bookJSON is a trimmed capture of the live /books/{asin} response shape.
const bookJSON = `{
  "asin": "B08G9PRS1K",
  "title": "Project Hail Mary",
  "subtitle": null,
  "authors": [{"asin": "B00G0WYW92", "name": "Andy Weir"}],
  "narrators": [{"name": "Ray Porter"}],
  "publisherName": "Audible Studios",
  "releaseDate": "2021-05-04T00:00:00.000Z",
  "runtimeLengthMin": 970,
  "language": "english",
  "image": "https://m.media-amazon.com/images/I/x.jpg",
  "description": "A lone astronaut. He must save the earth. Alone. Except he...",
  "summary": "<p>A lone astronaut.</p> <p>He must save the earth. Alone.</p><p>Except he &amp; his crew aren't &quot;alone&quot;.</p>",
  "seriesPrimary": {"asin": "X", "name": "Hail Mary", "position": "1"},
  "genres": [
    {"asin": "1", "name": "Science Fiction", "type": "genre"},
    {"asin": "2", "name": "Hard SF", "type": "tag"}
  ]
}`

const chaptersJSON = `{
  "asin": "B08G9PRS1K",
  "brandIntroDurationMs": 2043,
  "brandOutroDurationMs": 5061,
  "isAccurate": true,
  "runtimeLengthMs": 58252995,
  "chapters": [
    {"lengthMs": 11264, "startOffsetMs": 0, "startOffsetSec": 0, "title": "Opening Credits"},
    {"lengthMs": 2203838, "startOffsetMs": 11264, "startOffsetSec": 11, "title": "Chapter 1"}
  ]
}`

func TestGetBookParsesLiveShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/books/B08G9PRS1K" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("region") != "us" {
			t.Errorf("region not forwarded: %s", r.URL.RawQuery)
		}
		if r.Header.Get("User-Agent") != "audioborker/test" {
			t.Errorf("missing user agent, got %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(bookJSON))
	}))
	defer srv.Close()

	p := New(srv.URL, "audioborker/test")
	book, err := p.GetBook(context.Background(), "B08G9PRS1K", "us")
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "Project Hail Mary" || book.Year != "2021" ||
		book.SeriesName != "Hail Mary" || book.SeriesPosition != "1" {
		t.Errorf("bad parse: %+v", book)
	}
	if len(book.Genres) != 1 || book.Genres[0] != "Science Fiction" {
		t.Errorf("genres should keep type=genre only, got %v", book.Genres)
	}
	if len(book.Authors) != 1 || book.Authors[0] != "Andy Weir" {
		t.Errorf("authors: %v", book.Authors)
	}

	// The full summary must survive as plain text, and Blurb() must prefer it
	// over the provider's truncated teaser.
	wantSummary := "A lone astronaut.\n\nHe must save the earth. Alone.\n\nExcept he & his crew aren't \"alone\"."
	if book.Summary != wantSummary {
		t.Errorf("summary =\n%q\nwant\n%q", book.Summary, wantSummary)
	}
	if book.Blurb() != wantSummary {
		t.Errorf("Blurb() should prefer the full summary, got %q", book.Blurb())
	}
}

func TestBlurbFallsBackToDescription(t *testing.T) {
	b := metadata.Book{Description: "short teaser..."}
	if b.Blurb() != "short teaser..." {
		t.Errorf("got %q", b.Blurb())
	}
}

func TestHTMLToText(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"plain":                         "plain",
		"<p>one</p><p>two</p>":          "one\n\ntwo",
		"a<br>b":                        "a\n\nb",
		"<i>x</i> &amp; <b>y</b>":       "x & y",
		"&quot;q&quot; &#39;s&#39;":     `"q" 's'`,
		"<p>trailing space </p>":        "trailing space",
		"<ul><li>a</li><li>b</li></ul>": "a\n\nb",
	}
	for in, want := range cases {
		if got := htmlToText(in); got != want {
			t.Errorf("htmlToText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetChaptersParsesLiveShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/books/B08G9PRS1K/chapters" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(chaptersJSON))
	}))
	defer srv.Close()

	p := New(srv.URL, "audioborker/test")
	info, err := p.GetChapters(context.Background(), "B08G9PRS1K", "us")
	if err != nil {
		t.Fatal(err)
	}
	if info.RuntimeMs != 58252995 || !info.IsAccurate || len(info.Chapters) != 2 {
		t.Errorf("bad parse: %+v", info)
	}
	if info.Chapters[1].Title != "Chapter 1" || info.Chapters[1].StartMs != 11264 {
		t.Errorf("chapter parse: %+v", info.Chapters[1])
	}
}

func TestGetBook404IsNotFoundAndNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":{"code":"NOT_FOUND"}}`, http.StatusNotFound)
	}))
	defer srv.Close()

	p := New(srv.URL, "audioborker/test")
	_, err := p.GetBook(context.Background(), "B000000000", "uk")
	if !metadata.IsNotFound(err) {
		t.Fatalf("expected NotFound, got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("404 was retried %d times", calls.Load())
	}
}

func TestGetBookRetriesOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.Write([]byte(bookJSON))
	}))
	defer srv.Close()

	p := New(srv.URL, "audioborker/test")
	book, err := p.GetBook(context.Background(), "B08G9PRS1K", "us")
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if book.Title != "Project Hail Mary" || calls.Load() != 3 {
		t.Errorf("retries = %d, title = %q", calls.Load(), book.Title)
	}
}

func TestValidASIN(t *testing.T) {
	for asin, want := range map[string]bool{
		"B08G9PRS1K": true, "1774245191": true,
		"": false, "B08G9PRS1": false, "B08G9PRS1K2": false, "B08G9PRS1!": false,
	} {
		if got := ValidASIN(asin); got != want {
			t.Errorf("ValidASIN(%q) = %v, want %v", asin, got, want)
		}
	}
}
