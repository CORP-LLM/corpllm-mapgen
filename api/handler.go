package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/corpllm/mapgen/internal/terrain"
)

// Store holds generated terrains in memory.
type Store struct {
	mu   sync.RWMutex
	data map[string]*terrain.Terrain
}

func newStore() *Store {
	return &Store{data: make(map[string]*terrain.Terrain)}
}

func (s *Store) set(t *terrain.Terrain) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[t.Meta.ID] = t
}

func (s *Store) get(id string) (*terrain.Terrain, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data[id]
	return t, ok
}

// Handler holds app state.
type Handler struct {
	store *Store
}

func NewHandler() *Handler {
	return &Handler{store: newStore()}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// HandleGenerate handles POST /api/terrain/generate
func (h *Handler) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	var cfg terrain.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	t, err := terrain.Generate(&cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.store.set(t)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      t.Meta.ID,
		"terrain": t,
	})
}

// HandleGet handles GET /api/terrain/{id}
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request, id string) {
	t, ok := h.store.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "terrain not found: "+id)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// HandleRegenerate handles POST /api/terrain/{id}/regenerate
func (h *Handler) HandleRegenerate(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := h.store.get(id); !ok {
		writeError(w, http.StatusNotFound, "terrain not found: "+id)
		return
	}
	var cfg terrain.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	t, err := terrain.Generate(&cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Keep original ID.
	t.Meta.ID = id
	h.store.set(t)
	writeJSON(w, http.StatusOK, t)
}
