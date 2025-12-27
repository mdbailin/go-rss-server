package httputil

import (
    "encoding/json"
    "net/http"
)

func RespondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)

    jsonBytes, _ := json.Marshal(payload)
    jsonBytes = append(jsonBytes, '\n')
    w.Write(jsonBytes)
}

func RespondWithError(w http.ResponseWriter, code int, msg string) {
    RespondWithJSON(w, code, map[string]string{"error": msg})
}
