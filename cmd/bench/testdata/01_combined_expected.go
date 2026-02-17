package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// User represents a user in the system.
type User struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	Active    bool      `json:"active"`
}

// UserStore manages user persistence.
type UserStore struct {
	users  map[int]*User
	nextID int
}

// NewUserStore creates an empty user store.
func NewUserStore() *UserStore {
	return &UserStore{
		users:  make(map[int]*User),
		nextID: 1,
	}
}

// ErrorResponse is returned when a request fails.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{
		Code:    status,
		Message: msg,
	})
}

// HandleListUsers returns all users.
func (s *UserStore) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}

	log.Printf("listing %d users", len(users))
	writeJSON(w, http.StatusOK, users)
}

// HandleRegisterUser validates and creates a new user from the request body.
func (s *UserStore) HandleRegisterUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)

	if input.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(input.Username) < 3 {
		writeError(w, http.StatusBadRequest, "username must be at least 3 characters")
		return
	}
	if input.Email == "" || !strings.Contains(input.Email, "@") {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}

	for _, u := range s.users {
		if u.Username == input.Username {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
	}

	user := &User{
		ID:        s.nextID,
		Username:  input.Username,
		Email:     input.Email,
		CreatedAt: time.Now(),
		Active:    true,
	}
	s.users[user.ID] = user
	s.nextID++

	log.Printf("registered user %d: %s", user.ID, user.Username)
	writeJSON(w, http.StatusCreated, user)
}

// HandleGetUser retrieves a single user by ID.
func (s *UserStore) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing id parameter")
		return
	}

	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	user, ok := s.users[id]
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// HandleDeleteUser removes a user by ID.
func (s *UserStore) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing id parameter")
		return
	}

	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if _, ok := s.users[id]; !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	delete(s.users, id)
	log.Printf("deleted user %d", id)
	w.WriteHeader(http.StatusNoContent)
}

// HandleUpdateUser modifies an existing user.
func (s *UserStore) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing id parameter")
		return
	}

	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	user, ok := s.users[id]
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if input.Username != "" {
		user.Username = input.Username
	}
	if input.Email != "" {
		user.Email = input.Email
	}

	log.Printf("updated user %d", id)
	writeJSON(w, http.StatusOK, user)
}

// RegisterRoutes sets up the HTTP routes.
func (s *UserStore) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/users", s.HandleListUsers)
	mux.HandleFunc("/users/create", s.HandleRegisterUser)
	mux.HandleFunc("/users/get", s.HandleGetUser)
	mux.HandleFunc("/users/delete", s.HandleDeleteUser)
	mux.HandleFunc("/users/update", s.HandleUpdateUser)
}
