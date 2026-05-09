package monitoring

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
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

const jwtSecretEnv = "HEALTHOPS_JWT_SECRET"

func InitJWTSecretFromEnv() error {
	secret := strings.TrimSpace(os.Getenv(jwtSecretEnv))
	if secret == "" {
		return fmt.Errorf("%s is required", jwtSecretEnv)
	}
	if len([]byte(secret)) < 32 {
		return fmt.Errorf("%s must be at least 32 bytes", jwtSecretEnv)
	}
	jwtSecret = []byte(secret)
	return nil
}

// InitJWTSecret prepares the JWT secret. The dataDir argument is ignored and
// kept only while older constructors are removed from the runtime path.
func InitJWTSecret(dataDir string) {
	if err := InitJWTSecretFromEnv(); err != nil {
		panic(err)
	}
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
