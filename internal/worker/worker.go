package worker

import (
    "context"
    "errors"
    "log"
    "strings"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/lib/pq"

    "github.com/mdbailin/go-rss-server/internal/database"
    "github.com/mdbailin/go-rss-server/internal/rss"
)

func RunFeedWorker(db *database.Queries, interval time.Duration, batchSize int32) {
    log.Println("worker: starting feed worker...")

    for {
        ctx := context.Background()

        feeds, err := db.GetNextFeedsToFetch(ctx, batchSize)
        if err != nil {
            log.Printf("worker: GetNextFeedsToFetch error: %v", err)
            time.Sleep(interval)
            continue
        }

        if len(feeds) == 0 {
            log.Println("worker: no feeds to fetch")
            time.Sleep(interval)
            continue
        }

        log.Printf("worker: fetching %d feeds...", len(feeds))

        var wg sync.WaitGroup

        for _, f := range feeds {
            feed := f // avoid any loop-var capture weirdness
            wg.Add(1)

            go func() {
                defer wg.Done()
                processFeed(ctx, db, feed)
            }()
        }

        wg.Wait()
        log.Println("worker: batch complete")
        time.Sleep(interval)
    }
}

func processFeed(ctx context.Context, db *database.Queries, feed database.Feed) {
    log.Printf("worker: fetching feed %s (%s)", feed.Name, feed.Url)

    parsed, err := rss.FetchRSSFeed(ctx, feed.Url)
    if err != nil {
        log.Printf("worker: error fetching %s: %v", feed.Url, err)
        return
    }

    for _, item := range parsed.Channel.Items {
        title := strings.TrimSpace(item.Title)
        url := strings.TrimSpace(item.Link)

        if title == "" || url == "" {
            continue
        }

        // Parse pub date; fall back to now if we can't parse
        pubTime, err := time.Parse(time.RFC1123Z, item.PubDate)
        if err != nil {
            pubTime = time.Now().UTC()
        }

        now := time.Now().UTC()
        _, err = db.CreatePost(ctx, database.CreatePostParams{
            ID:          uuid.New(),
            CreatedAt:   now,
            UpdatedAt:   now,
            Title:       title,
            Url:         url,
            Description: item.Description,
            PublishedAt: pubTime,
            FeedID:      feed.ID,
        })
        if err != nil {
            var pqErr *pq.Error
            if errors.As(err, &pqErr) && pqErr.Code == "23505" {
                // duplicate URL – we’ve already stored this post; skip
                log.Printf("worker: duplicate post url=%s, skipping", url)
                continue
            }

            log.Printf("worker: error creating post for feed %s: %v", feed.ID, err)
            continue
        }

        log.Printf("worker: stored post %q from feed %s", title, feed.Name)
    }

    if err := db.MarkFeedFetched(ctx, feed.ID); err != nil {
        log.Printf("worker: error marking feed %s fetched: %v", feed.ID, err)
    } else {
        log.Printf("worker: marked feed %s as fetched", feed.Name)
    }
}
