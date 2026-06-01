package feedutil

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// FeedEntry represents a parsed feed entry
type FeedEntry struct {
	Name string
	URL  string
	Tags []string
}

// RawOutline represents an OPML outline element
type RawOutline struct {
	XMLName  xml.Name
	Text     string       `xml:"text,attr"`
	Title    string       `xml:"title,attr"`
	XMLURL   string       `xml:"xmlUrl,attr"`
	Outlines []RawOutline `xml:"outline"`
}

// ParseOPML parses OPML XML to extract RSS feed outlines
func ParseOPML(data []byte) ([]FeedEntry, error) {
	var opml struct {
		XMLName xml.Name `xml:"opml"`
		Body    struct {
			Outlines []RawOutline `xml:"outline"`
		} `xml:"body"`
	}

	if err := xml.Unmarshal(data, &opml); err != nil {
		return nil, fmt.Errorf("opml parse: %w", err)
	}

	var feeds []FeedEntry
	collectOutlines(opml.Body.Outlines, &feeds)
	return feeds, nil
}

func collectOutlines(outlines []RawOutline, feeds *[]FeedEntry) {
	for _, o := range outlines {
		if o.XMLURL != "" {
			name := o.Title
			if name == "" {
				name = o.Text
			}
			if name == "" {
				if u, _ := url.Parse(o.XMLURL); u != nil {
					name = u.Host
				}
			}
			*feeds = append(*feeds, FeedEntry{
				Name: SanitizeName(name),
				URL:  o.XMLURL,
				Tags: []string{},
			})
		}
		if len(o.Outlines) > 0 {
			collectOutlines(o.Outlines, feeds)
		}
	}
}

// ParseURLList parses a plain text file with one URL per line
func ParseURLList(data []byte) ([]FeedEntry, error) {
	var feeds []FeedEntry
	urlRE := regexp.MustCompile(`https?://[^\s]+`)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matches := urlRE.FindAllString(line, -1)
		for _, match := range matches {
			u, err := url.Parse(match)
			if err != nil {
				continue
			}
			name := u.Host
			feeds = append(feeds, FeedEntry{
				Name: SanitizeName(name),
				URL:  match,
				Tags: []string{},
			})
		}
	}
	return feeds, nil
}

// SanitizeName converts a string to a valid feed name (lowercase, alphanumeric, underscores)
func SanitizeName(name string) string {
	result := strings.ToLower(name)
	result = regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(result, "_")
	result = strings.Trim(result, "_")
	if result == "" {
		result = "feed"
	}
	return result
}

// DetectFormat detects if data is OPML or plain URL list
func DetectFormat(data []byte) string {
	content := string(data)
	if strings.Contains(content, "<opml") || strings.Contains(content, "<?xml") {
		return "opml"
	}
	return "url_list"
}
