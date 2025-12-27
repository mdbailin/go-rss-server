package rss

import (
    "context"
    "encoding/xml"
    "fmt"
    "net/http"
)

type RSSFeed struct {
    Channel RSSChannel `xml:"channel"`
}

type RSSChannel struct {
    Title string    `xml:"title"`
    Items []RSSItem `xml:"item"`
}

type RSSItem struct {
    Title       string `xml:"title"`
    Link        string `xml:"link"`
    Description string `xml:"description"`
    PubDate     string `xml:"pubDate"`
}

// FetchRSSFeed fetches & parses a remote RSS feed URL
func FetchRSSFeed(ctx context.Context, feedURL string) (*RSSFeed, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("do request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }

    var rss RSSFeed
    decoder := xml.NewDecoder(resp.Body)
    if err := decoder.Decode(&rss); err != nil {
        return nil, fmt.Errorf("decode rss: %w", err)
    }

    return &rss, nil
}

func DebugTestFetchRSS() {
    ctx := context.Background()
    url := "https://blog.boot.dev/index.xml"

    rss, err := FetchRSSFeed(ctx, url)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    fmt.Printf("Fetched RSS: %s\n", rss.Channel.Title)
    for i, item := range rss.Channel.Items {
        if i >= 5 { break }
        fmt.Printf(" - %s\n", item.Title)
    }
}
