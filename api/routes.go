package api

import (
	"net/http"
	"strings"
)

// RegisterRoutes wires all endpoints onto mux with CORS middleware.
func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.Handle("/api/terrain/generate", cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		h.HandleGenerate(w, r)
	})))

	mux.Handle("/api/terrain/", cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// /api/terrain/{id} or /api/terrain/{id}/regenerate
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/terrain/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			writeError(w, http.StatusBadRequest, "missing terrain id")
			return
		}
		id := parts[0]
		if len(parts) == 2 && parts[1] == "regenerate" {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "POST required")
				return
			}
			h.HandleRegenerate(w, r, id)
			return
		}
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		h.HandleGet(w, r, id)
	})))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		next.ServeHTTP(w, r)
	})
}
