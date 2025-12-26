package main

import (
    "fmt"
    "log"
    "net/http"
    "os"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/cors"
    "github.com/joho/godotenv"
    "encoding/json"
)

func main() {
    // Load environment variables
    godotenv.Load()

    port := os.Getenv("PORT")
    fmt.Println("Server starting on port:", port)

    r := chi.NewRouter()

    // Apply CORS middleware
    r.Use(cors.Handler(cors.Options{
        AllowedOrigins:   []string{"*"}, // allow all origins now
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"*"},
        AllowCredentials: false,
    }))

    // Create /v1 subrouter
    r.Route("/v1", func(v1 chi.Router) {
	 v1.Get("/readiness", func(w http.ResponseWriter, r *http.Request) {
       		 respondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
    	})
	v1.Get("/err", func(w http.ResponseWriter, r *http.Request) {
    		respondWithError(w, http.StatusInternalServerError, "Internal Server Error")
	})

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
