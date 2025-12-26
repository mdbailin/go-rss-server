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

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/cors"
    "github.com/joho/godotenv"
    "github.com/google/uuid"
    _ "github.com/lib/pq"

    "github.com/mdbailin/go-rss-server/internal/database"
)

type apiConfig struct {
    DB *database.Queries
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

    fmt.Println("Connected to DB!")
    fmt.Println("Server starting on port:", port)

    r := chi.NewRouter()

    r.Use(cors.Handler(cors.Options{
        AllowedOrigins:   []string{"*"},
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"*"},
        AllowCredentials: false,
    }))

    r.Route("/v1", func(v1 chi.Router) {
        v1.Get("/readiness", func(w http.ResponseWriter, r *http.Request) {
            respondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
        })
        v1.Get("/err", func(w http.ResponseWriter, r *http.Request) {
            respondWithError(w, http.StatusInternalServerError, "Internal Server Error")
        })

	v1.Post("/users", cfg.handleCreateUser)

	v1.Post("/feeds", cfg.middlewareAuth(cfg.handleCreateFeed))
    })

    srv := &http.Server{
        Addr:    ":" + port,
        Handler: r,
    }

    log.Fatal(srv.ListenAndServe())
}

// respondWithJSON writes status + JSON payload
func respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)

    jsonBytes, _ := json.Marshal(payload)
    jsonBytes = append(jsonBytes, byte('\n'))
    w.Write(jsonBytes)
}

// respondWithError is a helper that formats an error JSON
func respondWithError(w http.ResponseWriter, code int, msg string) {
    respondWithJSON(w, code, map[string]string{"error": msg})
}

type authedHandler func(http.ResponseWriter, *http.Request, database.User)

func (cfg *apiConfig) middlewareAuth(handler authedHandler) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        authHeader := r.Header.Get("Authorization")

        const prefix = "ApiKey "
        if !strings.HasPrefix(authHeader, prefix) {
            respondWithError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
            return
        }

        apiKey := strings.TrimPrefix(authHeader, prefix)
        if apiKey == "" {
            respondWithError(w, http.StatusUnauthorized, "invalid api key")
            return
        }

        user, err := cfg.DB.GetUserByAPIKey(r.Context(), apiKey)
        if err != nil {
            respondWithError(w, http.StatusUnauthorized, "invalid api key or user missing")
            return
        }

        handler(w, r, user)
    }
}

func (cfg *apiConfig) handleCreateUser(w http.ResponseWriter, r *http.Request) {
    type requestBody struct {
        Name string `json:"name"`
    }

    decoder := json.NewDecoder(r.Body)
    params := requestBody{}
    if err := decoder.Decode(&params); err != nil {
        respondWithError(w, http.StatusBadRequest, "invalid JSON")
        return
    }

    if params.Name == "" {
        respondWithError(w, http.StatusBadRequest, "name is required")
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
        respondWithError(w, http.StatusInternalServerError, "could not create user")
        return
    }

    respondWithJSON(w, http.StatusCreated, user)
}

func (cfg *apiConfig) handleCreateFeed(w http.ResponseWriter, r *http.Request, user database.User) {
    type requestBody struct {
        Name string `json:"name"`
        URL  string `json:"url"`
    }

    decoder := json.NewDecoder(r.Body)
    params := requestBody{}
    if err := decoder.Decode(&params); err != nil {
        respondWithError(w, http.StatusBadRequest, "invalid JSON")
        return
    }

    if params.Name == "" || params.URL == "" {
        respondWithError(w, http.StatusBadRequest, "name and url are required")
        return
    }

    id := uuid.New()
    now := time.Now().UTC()

    feed, err := cfg.DB.CreateFeed(r.Context(), database.CreateFeedParams{
        ID:        id,
        CreatedAt: now,
        UpdatedAt: now,
        Name:      params.Name,
        Url:       params.URL,
        UserID:    user.ID,
    })
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "could not create feed")
        return
    }

    respondWithJSON(w, http.StatusCreated, feed)
}
