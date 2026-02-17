package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// Item represents a stored item.
type Item struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
}

// Store provides item storage operations.
type Store struct {
	items map[string]*Item
}

// NewStore creates an empty store.
func NewStore() *Store {
	return &Store{items: make(map[string]*Item)}
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// HandleCreate adds a new item to the store.
func (s *Store) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var item Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if item.ID == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	if _, exists := s.items[item.ID]; exists {
		respondError(w, http.StatusConflict, "item already exists")
		return
	}

	s.items[item.ID] = &item
	log.Printf("created item %s", item.ID)
	respondJSON(w, http.StatusCreated, item)
}

// HandleRead retrieves an item by ID.
func (s *Store) HandleRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	item, exists := s.items[id]
	if !exists {
		respondError(w, http.StatusNotFound, "item not found")
		return
	}

	log.Printf("read item %s", id)
	respondJSON(w, http.StatusOK, item)
}

// HandleUpdate modifies an existing item.
func (s *Store) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		respondError(w, http.StatusUnauthorized, "authorization required")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	item, exists := s.items[id]
	if !exists {
		respondError(w, http.StatusNotFound, "item not found")
		return
	}

	var update Item
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if update.Name != "" {
		item.Name = update.Name
	}
	if update.Description != "" {
		item.Description = update.Description
	}
	if update.Quantity > 0 {
		item.Quantity = update.Quantity
	}

	log.Printf("updated item %s", id)
	respondJSON(w, http.StatusOK, item)
}

// HandleDelete removes an item by ID.
func (s *Store) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	_, exists := s.items[id]
	if !exists {
		respondError(w, http.StatusNotFound, "item not found")
		return
	}

	delete(s.items, id)
	log.Printf("deleted item %s", id)
	w.WriteHeader(http.StatusNoContent)
}

// HandleList returns all items in the store.
func (s *Store) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	items := make([]*Item, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}

	log.Printf("listed %d items", len(items))
	respondJSON(w, http.StatusOK, items)
}

// RegisterHandlers registers all CRUD routes on the given mux.
func (s *Store) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/items/create", s.HandleCreate)
	mux.HandleFunc("/items/read", s.HandleRead)
	mux.HandleFunc("/items/update", s.HandleUpdate)
	mux.HandleFunc("/items/delete", s.HandleDelete)
	mux.HandleFunc("/items", s.HandleList)
}

// Stats returns a formatted string with store statistics.
func (s *Store) Stats() string {
	totalQty := 0
	for _, item := range s.items {
		totalQty += item.Quantity
	}
	return fmt.Sprintf("items: %d, total quantity: %d", len(s.items), totalQty)
}
