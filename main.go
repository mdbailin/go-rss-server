package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "time"
    "strings"
    "context"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/cors"
    "github.com/joho/godotenv"
    "github.com/google/uuid"
    _ "github.com/lib/pq"

    "github.com/mdbailin/go-rss-server/internal/database"
   // "github.com/mdbailin/go-rss-server/internal/rss"
    "github.com/mdbailin/go-rss-server/internal/worker"
    "github.com/mdbailin/go-rss-server/internal/httputil"
)

type apiConfig struct {
    DB *database.Queries
}

//debugging
func (cfg *apiConfig) debugTestNextFeeds() {
    ctx := context.Background()

    feeds, err := cfg.DB.GetNextFeedsToFetch(ctx, 10)
    if err != nil {
        log.Printf("GetNextFeedsToFetch error: %v", err)
        return
    }

    log.Printf("Next feeds to fetch (%d):", len(feeds))
    for _, f := range feeds {
        log.Printf("  id=%s name=%q last_fetched_at=%v", f.ID, f.Name, f.LastFetchedAt)
    }

    if len(feeds) == 0 {
        return
    }

    // Mark the first one as fetched
    first := feeds[0]
    if err := cfg.DB.MarkFeedFetched(ctx, first.ID); err != nil {
        log.Printf("MarkFeedFetched error: %v", err)
        return
    }
    log.Printf("Marked feed %s as fetched", first.ID)
}

type Feed struct {
    ID            uuid.UUID  `json:"id"`
    CreatedAt     time.Time  `json:"created_at"`
    UpdatedAt     time.Time  `json:"updated_at"`
    Name          string     `json:"name"`
    URL           string     `json:"url"`
    UserID        uuid.UUID  `json:"user_id"`
    LastFetchedAt *time.Time `json:"last_fetched_at"`
}

func main() {
    godotenv.Load()

    port := os.Getenv("PORT")
    dbURL := os.Getenv("DB_URL")

    db, err := sql.Open("postgres", dbURL)
    if err != nil {
        log.Fatalf("failed to open db: %v", err)
    }
    defer db.Close()

    dbQueries := database.New(db)

    cfg := apiConfig{
        DB: dbQueries,
    }

    //rss.DebugTestFetchRSS()
    go worker.RunFeedWorker(cfg.DB, time.Minute, 10)

    fmt.Println("Connected to DB!")
    fmt.Println("Server starting on port:", port)

   // cfg.debugTestNextFeeds()

    r := chi.NewRouter()

    r.Use(cors.Handler(cors.Options{
        AllowedOrigins:   []string{"*"},
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"*"},
        AllowCredentials: false,
    }))

    r.Route("/v1", func(v1 chi.Router) {
        v1.Get("/readiness", func(w http.ResponseWriter, r *http.Request) {
            httputil.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
        })
        v1.Get("/err", func(w http.ResponseWriter, r *http.Request) {
            httputil.RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
        })

	v1.Post("/users", cfg.handleCreateUser)

	v1.Post("/feeds", cfg.middlewareAuth(cfg.handleCreateFeed))

	v1.Get("/feeds", cfg.handleGetFeeds)

	v1.Get("/posts", cfg.handleGetPosts)

	v1.Get("/posts/{postID}", cfg.handleGetPostByID)

	v1.Post("/feed_follows", cfg.middlewareAuth(cfg.handleCreateFeedFollow))

	v1.Get("/feed_follows", cfg.middlewareAuth(cfg.handleGetFeedFollows))

	v1.Delete("/feed_follows/{feedFollowID}", cfg.middlewareAuth(cfg.handleDeleteFeedFollow))
    })

    srv := &http.Server{
        Addr:    ":" + port,
        Handler: r,
    }

    log.Fatal(srv.ListenAndServe())
}

type authedHandler func(http.ResponseWriter, *http.Request, database.User)

func (cfg *apiConfig) middlewareAuth(handler authedHandler) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        authHeader := r.Header.Get("Authorization")

        const prefix = "ApiKey "
        if !strings.HasPrefix(authHeader, prefix) {
            httputil.RespondWithError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
            return
        }

        apiKey := strings.TrimPrefix(authHeader, prefix)
        if apiKey == "" {
            httputil.RespondWithError(w, http.StatusUnauthorized, "invalid api key")
            return
        }

        user, err := cfg.DB.GetUserByAPIKey(r.Context(), apiKey)
        if err != nil {
            httputil.RespondWithError(w, http.StatusUnauthorized, "invalid api key or user missing")
            return
        }

        handler(w, r, user)
    }
}

func (cfg *apiConfig) handleGetPosts(w http.ResponseWriter, r *http.Request) {
    posts, err := cfg.DB.GetPosts(r.Context()) 
    if err != nil {
        httputil.RespondWithError(w, http.StatusInternalServerError, "could not get posts")
        return
    }
    httputil.RespondWithJSON(w, http.StatusOK, posts)
}

func (cfg *apiConfig) handleGetPostByID(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "postID")
    id, err := uuid.Parse(idStr)
    if err != nil {
        httputil.RespondWithError(w, http.StatusBadRequest, "invalid postID")
        return
    }

    post, err := cfg.DB.GetPost(r.Context(), id) // new sqlc query
    if err != nil {
        httputil.RespondWithError(w, http.StatusNotFound, "post not found")
        return
    }

    httputil.RespondWithJSON(w, http.StatusOK, post)
}

func (cfg *apiConfig) handleCreateUser(w http.ResponseWriter, r *http.Request) {
    type requestBody struct {
        Name string `json:"name"`
    }

    decoder := json.NewDecoder(r.Body)
    params := requestBody{}
    if err := decoder.Decode(&params); err != nil {
        httputil.RespondWithError(w, http.StatusBadRequest, "invalid JSON")
        return
    }

    if params.Name == "" {
        httputil.RespondWithError(w, http.StatusBadRequest, "name is required")
        return
    }

    id := uuid.New()
    apiKey := uuid.NewString()
    now := time.Now().UTC()

    user, err := cfg.DB.CreateUser(r.Context(), database.CreateUserParams{
        ID:        id,
        CreatedAt: now,
        UpdatedAt: now,
        Name:      params.Name,
        ApiKey:    apiKey,
    })
    if err != nil {
        httputil.RespondWithError(w, http.StatusInternalServerError, "could not create user")
        return
    }

    httputil.RespondWithJSON(w, http.StatusCreated, user)
}

func (cfg *apiConfig) handleCreateFeed(w http.ResponseWriter, r *http.Request, user database.User) {
    type requestBody struct {
        Name string `json:"name"`
        URL  string `json:"url"`
    }

    decoder := json.NewDecoder(r.Body)
    params := requestBody{}
    if err := decoder.Decode(&params); err != nil {
        httputil.RespondWithError(w, http.StatusBadRequest, "invalid JSON")
        return
    }

    if params.Name == "" || params.URL == "" {
        httputil.RespondWithError(w, http.StatusBadRequest, "name and url are required")
        return
    }

    now := time.Now().UTC()

    // 1. Create the feed
    feedID := uuid.New()
    feed, err := cfg.DB.CreateFeed(r.Context(), database.CreateFeedParams{
        ID:        feedID,
        CreatedAt: now,
        UpdatedAt: now,
        Name:      params.Name,
        Url:       params.URL,
        UserID:    user.ID,
    })
    if err != nil {
        httputil.RespondWithError(w, http.StatusInternalServerError, "could not create feed")
        return
    }

    // 2. Auto-create follow
    followID := uuid.New()
    follow, err := cfg.DB.CreateFeedFollow(r.Context(), database.CreateFeedFollowParams{
        ID:        followID,
        CreatedAt: now,
        UpdatedAt: now,
        FeedID:    feed.ID,
        UserID:    user.ID,
    })
    if err != nil {
        httputil.RespondWithError(w, http.StatusInternalServerError, "could not create feed follow")
        return
    }

    // 3. Return both
    type response struct {
        Feed       Feed       `json:"feed"`
        FeedFollow database.FeedFollow `json:"feed_follow"`
    }

    httputil.RespondWithJSON(w, http.StatusCreated, response{
        Feed:       databaseFeedToFeed(feed),
        FeedFollow: follow,
    })
}

func (cfg *apiConfig) handleGetFeeds(w http.ResponseWriter, r *http.Request) {
    feeds, err := cfg.DB.GetFeeds(r.Context())
    if err != nil {
        httputil.RespondWithError(w, http.StatusInternalServerError, "could not get feeds")
        return
    }

    httputil.RespondWithJSON(w, http.StatusOK, databaseFeedsToFeeds(feeds))
}

func (cfg *apiConfig) handleCreateFeedFollow(w http.ResponseWriter, r *http.Request, user database.User) {
    type requestBody struct {
        FeedID string `json:"feed_id"`
    }

    decoder := json.NewDecoder(r.Body)
    params := requestBody{}
    if err := decoder.Decode(&params); err != nil {
        httputil.RespondWithError(w, http.StatusBadRequest, "invalid JSON")
        return
    }

    if params.FeedID == "" {
        httputil.RespondWithError(w, http.StatusBadRequest, "feed_id is required")
        return
    }

    feedID, err := uuid.Parse(params.FeedID)
    if err != nil {
        httputil.RespondWithError(w, http.StatusBadRequest, "invalid feed_id")
        return
    }

    id := uuid.New()
    now := time.Now().UTC()

    follow, err := cfg.DB.CreateFeedFollow(r.Context(), database.CreateFeedFollowParams{
        ID:        id,
        CreatedAt: now,
        UpdatedAt: now,
        FeedID:    feedID,
        UserID:    user.ID,
    })
    if err != nil {
        httputil.RespondWithError(w, http.StatusInternalServerError, "could not create feed follow")
        return
    }

    httputil.RespondWithJSON(w, http.StatusCreated, follow)
}

func (cfg *apiConfig) handleGetFeedFollows(w http.ResponseWriter, r *http.Request, user database.User) {
    follows, err := cfg.DB.GetFeedFollowsForUser(r.Context(), user.ID)
    if err != nil {
        httputil.RespondWithError(w, http.StatusInternalServerError, "could not get feed follows")
        return
    }

    httputil.RespondWithJSON(w, http.StatusOK, follows)
}

func (cfg *apiConfig) handleDeleteFeedFollow(w http.ResponseWriter, r *http.Request, user database.User) {
    feedFollowIDStr := chi.URLParam(r, "feedFollowID")
    if feedFollowIDStr == "" {
        httputil.RespondWithError(w, http.StatusBadRequest, "feedFollowID is required")
        return
    }

    feedFollowID, err := uuid.Parse(feedFollowIDStr)
    if err != nil {
        httputil.RespondWithError(w, http.StatusBadRequest, "invalid feedFollowID")
        return
    }

    err = cfg.DB.DeleteFeedFollow(r.Context(), database.DeleteFeedFollowParams{
        ID:     feedFollowID,
        UserID: user.ID,
    })
    if err != nil {
        // could refine (e.g. 404), but 400/500 is OK for now
        httputil.RespondWithError(w, http.StatusInternalServerError, "could not delete feed follow")
        return
    }

    httputil.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func databaseFeedToFeed(f database.Feed) Feed {
    var lastFetched *time.Time
    if f.LastFetchedAt.Valid {
        t := f.LastFetchedAt.Time
        lastFetched = &t
    }

    return Feed{
        ID:            f.ID,
        CreatedAt:     f.CreatedAt,
        UpdatedAt:     f.UpdatedAt,
        Name:          f.Name,
        URL:           f.Url,
        UserID:        f.UserID,
        LastFetchedAt: lastFetched,
    }
}

func databaseFeedsToFeeds(feeds []database.Feed) []Feed {
    out := make([]Feed, 0, len(feeds))
    for _, f := range feeds {
        out = append(out, databaseFeedToFeed(f))
    }
    return out
}
