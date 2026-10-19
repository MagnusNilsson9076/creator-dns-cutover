package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dns := &InfraiDNS{
		BaseURL: defaultBaseURL,
		APIKey:  key,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /cutover", cutoverHandler(dns))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("creator DNS cutover listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}

func cutoverHandler(dns DNSWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req CutoverRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
			return
		}
		result, err := ApplyCutover(r.Context(), dns, req)
		if err != nil {
			status := http.StatusBadRequest
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				status = apiErr.Status
				if status < 400 || status >= 500 {
					status = http.StatusBadGateway
				}
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
