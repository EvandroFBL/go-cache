package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Handler provides HTTP handlers for the cache API.
type Handler struct {
	store *Store
}

// NewHandler creates a new HTTP handler with the given store.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes sets up the HTTP routes using Go 1.22+ enhanced mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /cache/{key}", h.handleGet)
	mux.HandleFunc("PUT /cache/{key}", h.handleSet)
	mux.HandleFunc("DELETE /cache/{key}", h.handleDelete)
	mux.HandleFunc("GET /cache", h.handleKeys)
	mux.HandleFunc("GET /stats", h.handleStats)
	mux.HandleFunc("GET /health", h.handleHealth)
}

// ErrorResponse is a standard error JSON shape.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// jsonError writes an error response.
func jsonError(w http.ResponseWriter, status int, errMsg string, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: errMsg, Message: msg})
}

// GET /cache/{key}
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		jsonError(w, http.StatusBadRequest, "missing_key", "key is required")
		return
	}

	userID := GetOrCreateUserID(w, r)
	nsKey := namespacedKey(userID, key)

	value, found := h.store.Get(nsKey)
	if !found {
		jsonError(w, http.StatusNotFound, "not_found", "key not found or expired")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"key":   key,
		"value": value,
	})
}

// SetRequest is the JSON body for PUT /cache/{key}.
type SetRequest struct {
	Value any    `json:"value"`
	TTL   string `json:"ttl"` // Duration string like "5m", "1h", "30s"
}

// PUT /cache/{key}
func (h *Handler) handleSet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		jsonError(w, http.StatusBadRequest, "missing_key", "key is required")
		return
	}

	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if req.Value == nil {
		jsonError(w, http.StatusBadRequest, "missing_value", "value is required")
		return
	}

	ttl := 5 * time.Minute // default
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid_ttl", "use Go duration format: 30s, 5m, 1h")
			return
		}
		ttl = parsed
	}

	userID := GetOrCreateUserID(w, r)
	nsKey := namespacedKey(userID, key)

	if err := h.store.Set(nsKey, req.Value, ttl); err != nil {
		jsonError(w, http.StatusConflict, "cache_full", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"key":     key,
		"ttl":     ttl.String(),
		"expires": time.Now().Add(ttl).Format(time.RFC3339),
	})
}

// DELETE /cache/{key}
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		jsonError(w, http.StatusBadRequest, "missing_key", "key is required")
		return
	}

	userID := GetOrCreateUserID(w, r)
	nsKey := namespacedKey(userID, key)

	deleted := h.store.Delete(nsKey)
	if !deleted {
		jsonError(w, http.StatusNotFound, "not_found", "key not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"key":     key,
		"deleted": true,
	})
}

// GET /cache — list all non-expired keys for the current user
func (h *Handler) handleKeys(w http.ResponseWriter, r *http.Request) {
	userID := GetOrCreateUserID(w, r)
	prefix := "user:" + userID + ":"

	allKeys := h.store.Keys()

	// Filter to only this user's keys and strip the prefix
	keys := make([]string, 0)
	for _, k := range allKeys {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, strings.TrimPrefix(k, prefix))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"keys":   keys,
		"count":  len(keys),
		"userID": userID,
	})
}

// GET /stats — cache statistics (global, not per-user)
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	snap := h.store.stats.Snapshot()
	total := snap.Hits + snap.Misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(snap.Hits) / float64(total) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"stats":    snap,
		"keys":     h.store.Len(),
		"hit_rate": fmt.Sprintf("%.1f%%", hitRate),
	})
}

// GET /health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"keys":   h.store.Len(),
	})
}
