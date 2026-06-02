package step1

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Feed represents a parsed RSS/Atom feed entry
type Feed struct {
	ID        int64
	Name      string
	URL       string
	Tags      []string
	Interval  string
	LastFetch time.Time
	LastBuild string
}

// Item represents a single feed item
type Item struct {
	Title       string
	URL         string
	Description string
	Content     string
	Published   time.Time
}

// Fetcher fetches RSS/Atom feeds
type Fetcher struct {
	httpClient *http.Client
}

// NewFetcher creates a new RSS fetcher
func NewFetcher() *Fetcher {
	return &Fetcher{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Fetch fetches and parses an RSS or Atom feed
func (f *Fetcher) Fetch(ctx context.Context, feed *Feed) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", "llm-wiki-pipeline/1.0")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return f.parse(feed.URL, body)
}

func (f *Fetcher) parse(url string, body []byte) ([]Item, error) {
	// Try RSS first, then Atom
	if items, err := f.parseRSS(body); err == nil && len(items) > 0 {
		return items, nil
	}
	if items, err := f.parseAtom(body); err == nil && len(items) > 0 {
		return items, nil
	}
	return nil, fmt.Errorf("unrecognized feed format")
}

func (f *Fetcher) parseRSS(body []byte) ([]Item, error) {
	var rss RSSFeed
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rss.Channel.Items))
	for _, i := range rss.Channel.Items {
		content := i.EncodedContent
		if content == "" {
			content = i.Description
		}
		items = append(items, Item{
			Title:       i.Title,
			URL:         i.Link,
			Description: i.Description,
			Content:     content,
			Published:   parseTime(i.PubDate),
		})
	}
	return items, nil
}

func (f *Fetcher) parseAtom(body []byte) ([]Item, error) {
	var atom AtomFeed
	if err := xml.Unmarshal(body, &atom); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(atom.Entries))
	for _, e := range atom.Entries {
		link := ""
		for _, l := range e.Links {
			if l.Rel == "alternate" || l.Rel == "" {
				link = l.Href
				break
			}
		}
		content := e.Content
		if content == "" {
			content = e.Summary
		}
		items = append(items, Item{
			Title:       e.Title,
			URL:         link,
			Description: e.Summary,
			Content:     content,
			Published:   parseTime(e.Published),
		})
	}
	return items, nil
}

// SaveToFile persists a feed item to basePath/YYYY-MM-DD/source_name.md
func SaveToFile(item *Item, feedName, basePath string) (string, error) {
	date := item.Published.Format("2006-01-02")
	if item.Published.IsZero() {
		date = time.Now().Format("2006-01-02")
	}

	dir := filepath.Join(basePath, date)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	// Build filename from title
	safeName := sanitizeFilename(item.Title)
	if safeName == "" {
		safeName = fmt.Sprintf("item_%d", time.Now().UnixNano())
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.md", safeName))

	// Write markdown
	content := fmt.Sprintf("---\ntitle: %s\nurl: %s\npublished: %s\nsource: %s\n---\n\n# %s\n\n%s",
		item.Title, item.URL, item.Published.Format(time.RFC3339), feedName, item.Title, item.Content)

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return filename, nil
}

// ContentHash computes SHA-256 hash of title+content for dedup
func ContentHash(item *Item) string {
	h := sha256.New()
	h.Write([]byte(item.Title + "\n" + item.Content))
	return hex.EncodeToString(h.Sum(nil))
}

func sanitizeFilename(name string) string {
	repl := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "-", "?", "-",
		`"`, "-", "<", "-", ">", "-", "|", "-",
	)
	safe := repl.Replace(name)
	return strings.Trim(safe, " .-")
}

func parseTime(s string) time.Time {
	formats := []string{
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// --- XML structs ---

type RSSFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel RSSChannel `xml:"channel"`
}

type RSSChannel struct {
	Items []RSSItem `xml:"item"`
}

type RSSItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description string   `xml:"description"`
	EncodedContent string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	PubDate     string   `xml:"pubDate"`
}

type AtomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []AtomEntry `xml:"entry"`
}

type AtomEntry struct {
	Title     string     `xml:"title"`
	Links     []AtomLink `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Published string     `xml:"published"`
}

type AtomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}
