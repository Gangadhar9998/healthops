package monitoring

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

const (
	RoleAdmin = "admin"
	RoleOps   = "ops"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	DisplayName  string    `json:"displayName,omitempty"`
	Email        string    `json:"email,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type userJSON struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"passwordHash"`
	Role         string    `json:"role"`
	DisplayName  string    `json:"displayName,omitempty"`
	Email        string    `json:"email,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type CreateUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
}

type UpdateUserRequest struct {
	Password    *string `json:"password,omitempty"`
	Role        *string `json:"role,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	Email       *string `json:"email,omitempty"`
}

// UserStoreBackend defines the user-management operations needed by auth and APIs.
type UserStoreBackend interface {
	Authenticate(username, password string) (*User, error)
	IsUsingDefaultCredentials() bool
	List() []User
	Get(id string) (*User, bool)
	Create(req CreateUserRequest) (*User, error)
	Update(id string, req UpdateUserRequest) (*User, error)
	Delete(id string) error
}

// ---------------------------------------------------------------------------
// JWT (minimal HMAC-SHA256)
// ---------------------------------------------------------------------------

type JWTClaims struct {
	UserID   string `json:"sub"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
}

var jwtSecret []byte

func initJWTSecret(dataDir string) {
	keyPath := dataDir + "/.jwt_secret"
	if data, err := os.ReadFile(keyPath); err == nil && len(data) >= 32 {
		jwtSecret = data
		return
	}
	jwtSecret = make([]byte, 32)
	if _, err := rand.Read(jwtSecret); err != nil {
		panic("failed to generate JWT secret: " + err.Error())
	}
	_ = os.WriteFile(keyPath, jwtSecret, 0600)
}

// InitJWTSecret prepares the JWT secret for non-file-backed user stores.
func InitJWTSecret(dataDir string) {
	initJWTSecret(dataDir)
}

func signJWT(claims JWTClaims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	msg := header + "." + payloadEnc

	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(msg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return msg + "." + sig, nil
}

func verifyJWT(token string) (*JWTClaims, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}

	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

// ExtractJWTClaims extracts JWT claims from the Authorization header
// or from a "token" query parameter (for EventSource/SSE which cannot set headers).
func ExtractJWTClaims(r *http.Request) *JWTClaims {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		claims, err := verifyJWT(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			return nil
		}
		return claims
	}
	// Fallback: check query parameter (used by EventSource for SSE)
	if tok := r.URL.Query().Get("token"); tok != "" {
		claims, err := verifyJWT(tok)
		if err != nil {
			return nil
		}
		return claims
	}
	return nil
}

// ---------------------------------------------------------------------------
// User Store (file-backed)
// ---------------------------------------------------------------------------

type UserStore struct {
	mu    sync.RWMutex
	users map[string]*User // keyed by ID
	path  string
}

func NewUserStore(dataDir string) (*UserStore, error) {
	initJWTSecret(dataDir)

	path := dataDir + "/users.json"
	store := &UserStore{
		users: make(map[string]*User),
		path:  path,
	}

	if data, err := os.ReadFile(path); err == nil {
		var records []userJSON
		if err := json.Unmarshal(data, &records); err == nil {
			for _, r := range records {
				u := &User{
					ID:           r.ID,
					Username:     r.Username,
					PasswordHash: r.PasswordHash,
					Role:         r.Role,
					DisplayName:  r.DisplayName,
					Email:        r.Email,
					CreatedAt:    r.CreatedAt,
					UpdatedAt:    r.UpdatedAt,
				}
				store.users[u.ID] = u
			}
		}
	}

	// Seed default admin if no users exist
	if len(store.users) == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		now := time.Now().UTC()
		store.users["admin"] = &User{
			ID:           "admin",
			Username:     "admin",
			PasswordHash: string(hash),
			Role:         RoleAdmin,
			DisplayName:  "Administrator",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		store.save()
	}

	return store, nil
}

// save persists users to disk. Caller must hold s.mu (read or write).
func (s *UserStore) save() {
	records := make([]userJSON, 0, len(s.users))
	for _, u := range s.users {
		records = append(records, userJSON{
			ID:           u.ID,
			Username:     u.Username,
			PasswordHash: u.PasswordHash,
			Role:         u.Role,
			DisplayName:  u.DisplayName,
			Email:        u.Email,
			CreatedAt:    u.CreatedAt,
			UpdatedAt:    u.UpdatedAt,
		})
	}

	data, _ := json.MarshalIndent(records, "", "  ")
	_ = os.WriteFile(s.path, data, 0600)
}

func (s *UserStore) Authenticate(username, password string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		if u.Username == username {
			if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
				return nil, fmt.Errorf("invalid credentials")
			}
			return u, nil
		}
	}
	return nil, fmt.Errorf("invalid credentials")
}

// IsUsingDefaultCredentials checks if the only user is the default admin with default password.
func (s *UserStore) IsUsingDefaultCredentials() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.users) != 1 {
		return false
	}
	u, ok := s.users["admin"]
	if !ok {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("admin")) == nil
}

func (s *UserStore) List() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]User, 0, len(s.users))
	for _, u := range s.users {
		result = append(result, *u)
	}
	return result
}

func (s *UserStore) Get(id string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return nil, false
	}
	copy := *u
	return &copy, true
}

func (s *UserStore) Create(req CreateUserRequest) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check duplicate username
	for _, u := range s.users {
		if u.Username == req.Username {
			return nil, fmt.Errorf("username already exists")
		}
	}

	if req.Role != RoleAdmin && req.Role != RoleOps {
		return nil, fmt.Errorf("role must be 'admin' or 'ops'")
	}

	if len(req.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	id := strings.ToLower(req.Username)
	now := time.Now().UTC()
	u := &User{
		ID:           id,
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         req.Role,
		DisplayName:  req.DisplayName,
		Email:        req.Email,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.users[id] = u
	s.save()
	return u, nil
}

func (s *UserStore) Update(id string, req UpdateUserRequest) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}

	if req.Password != nil && len(*req.Password) >= 4 {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		u.PasswordHash = string(hash)
	}

	if req.Role != nil {
		if *req.Role != RoleAdmin && *req.Role != RoleOps {
			return nil, fmt.Errorf("role must be 'admin' or 'ops'")
		}
		u.Role = *req.Role
	}

	if req.DisplayName != nil {
		if req.Email != nil {
			u.Email = *req.Email
		}

		u.DisplayName = *req.DisplayName
	}

	if req.Email != nil {
		u.Email = *req.Email
	}

	u.UpdatedAt = time.Now().UTC()
	s.save()

	copy := *u
	return &copy, nil
}

func (s *UserStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[id]; !ok {
		return fmt.Errorf("user not found")
	}

	// Prevent deleting the last admin
	adminCount := 0
	for _, u := range s.users {
		if u.Role == RoleAdmin {
			adminCount++
		}
	}
	if s.users[id].Role == RoleAdmin && adminCount <= 1 {
		return fmt.Errorf("cannot delete the last admin user")
	}

	delete(s.users, id)
	s.save()
	return nil
}

// ---------------------------------------------------------------------------
// HTTP Handlers
// ---------------------------------------------------------------------------

type UserAPIHandler struct {
	store UserStoreBackend
}

func NewUserAPIHandler(store UserStoreBackend) *UserAPIHandler {
	return &UserAPIHandler{store: store}
}

func (h *UserAPIHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}

	user, err := h.store.Authenticate(req.Username, req.Password)
	if err != nil {
		WriteAPIError(w, http.StatusUnauthorized, fmt.Errorf("invalid credentials"))
		return
	}

	token, err := signJWT(JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Iat:      time.Now().Unix(),
		Exp:      time.Now().Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("generate token: %w", err))
		return
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(LoginResponse{
		Token: token,
		User:  *user,
	}))
}

func (h *UserAPIHandler) HandleUsers(w http.ResponseWriter, r *http.Request) {
	claims := ExtractJWTClaims(r)
	if claims == nil {
		WriteAPIError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		users := h.store.List()
		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(users))

	case http.MethodPost:
		if claims.Role != RoleAdmin {
			WriteAPIError(w, http.StatusForbidden, fmt.Errorf("admin role required"))
			return
		}

		var req CreateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
			return
		}

		user, err := h.store.Create(req)
		if err != nil {
			WriteAPIError(w, http.StatusBadRequest, err)
			return
		}

		WriteAPIResponse(w, http.StatusCreated, NewAPIResponse(user))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *UserAPIHandler) HandleUserByID(w http.ResponseWriter, r *http.Request) {
	claims := ExtractJWTClaims(r)
	if claims == nil {
		WriteAPIError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if id == "" {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("user id required"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		user, ok := h.store.Get(id)
		if !ok {
			WriteAPIError(w, http.StatusNotFound, fmt.Errorf("user not found"))
			return
		}
		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(user))

	case http.MethodPut:
		if claims.Role != RoleAdmin {
			WriteAPIError(w, http.StatusForbidden, fmt.Errorf("admin role required"))
			return
		}

		var req UpdateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
			return
		}

		user, err := h.store.Update(id, req)
		if err != nil {
			WriteAPIError(w, http.StatusBadRequest, err)
			return
		}

		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(user))

	case http.MethodDelete:
		if claims.Role != RoleAdmin {
			WriteAPIError(w, http.StatusForbidden, fmt.Errorf("admin role required"))
			return
		}

		if err := h.store.Delete(id); err != nil {
			WriteAPIError(w, http.StatusBadRequest, err)
			return
		}

		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"deleted": id}))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *UserAPIHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims := ExtractJWTClaims(r)
	if claims == nil {
		WriteAPIError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
		return
	}

	user, ok := h.store.Get(claims.UserID)
	if !ok {
		WriteAPIError(w, http.StatusNotFound, fmt.Errorf("user not found"))
		return
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]interface{}{
		"user":        user,
		"authEnabled": true,
	}))
}
