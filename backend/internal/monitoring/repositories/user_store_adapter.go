package repositories

import (
	"context"
	"fmt"
	"strings"

	"medics-health-check/backend/internal/monitoring"
)

// UserStoreAdapter adapts MongoUserRepository to work with monitoring.UserStoreBackend.
type UserStoreAdapter struct {
	repo *MongoUserRepository
}

// NewUserStoreAdapter creates a new adapter for MongoUserRepository
func NewUserStoreAdapter(repo *MongoUserRepository) *UserStoreAdapter {
	if repo == nil {
		panic("mongo repository cannot be nil")
	}
	return &UserStoreAdapter{repo: repo}
}

// Authenticate validates username and password against MongoDB
func (a *UserStoreAdapter) Authenticate(username, password string) (*monitoring.User, error) {
	ctx := context.Background()
	u, err := a.repo.Authenticate(ctx, username, password)
	if err != nil {
		return nil, err
	}
	return a.convertToMonitoringUser(u), nil
}

// IsUsingDefaultCredentials reports whether the adapter is still using a known default password.
func (a *UserStoreAdapter) IsUsingDefaultCredentials() bool {
	return false
}

// List returns all users from MongoDB
func (a *UserStoreAdapter) List() []monitoring.User {
	ctx := context.Background()
	users, err := a.repo.List(ctx)
	if err != nil {
		return []monitoring.User{}
	}

	result := make([]monitoring.User, len(users))
	for i, u := range users {
		if converted := a.convertToMonitoringUser(&u); converted != nil {
			result[i] = *converted
		}
	}
	return result
}

// Get retrieves a user by ID (username) from MongoDB
func (a *UserStoreAdapter) Get(id string) (*monitoring.User, bool) {
	if id == "" {
		return nil, false
	}
	ctx := context.Background()
	u, err := a.repo.FindByUsername(ctx, id)
	if err != nil {
		return nil, false
	}
	return a.convertToMonitoringUser(u), true
}

// Create creates a new user in MongoDB
func (a *UserStoreAdapter) Create(req monitoring.CreateUserRequest) (*monitoring.User, error) {
	ctx := context.Background()

	repoUser := &User{
		Username:    req.Username,
		Password:    req.Password,
		Role:        mapRoleToRepository(req.Role),
		DisplayName: req.DisplayName,
		Email:       req.Email,
	}

	if err := a.repo.Create(ctx, repoUser); err != nil {
		return nil, err
	}

	created, err := a.repo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("user created but failed to retrieve: %w", err)
	}

	return a.convertToMonitoringUser(created), nil
}

// Update updates an existing user in MongoDB
func (a *UserStoreAdapter) Update(id string, req monitoring.UpdateUserRequest) (*monitoring.User, error) {
	if id == "" {
		return nil, fmt.Errorf("user id required")
	}

	ctx := context.Background()

	repoUser := &User{}
	if req.Role != nil {
		repoUser.Role = mapRoleToRepository(*req.Role)
	}
	if req.Email != nil {
		repoUser.Email = *req.Email
	}
	if req.DisplayName != nil {
		repoUser.DisplayName = *req.DisplayName
	}
	if req.Password != nil {
		repoUser.Password = *req.Password
	}

	username := id
	if err := a.repo.Update(ctx, username, repoUser); err != nil {
		return nil, err
	}

	updated, err := a.repo.FindByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("user updated but failed to retrieve: %w", err)
	}

	return a.convertToMonitoringUser(updated), nil
}

// Delete removes a user from MongoDB
func (a *UserStoreAdapter) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("user id required")
	}
	ctx := context.Background()
	return a.repo.Delete(ctx, id)
}

// convertToMonitoringUser converts repositories.User to monitoring.User
func (a *UserStoreAdapter) convertToMonitoringUser(u *User) *monitoring.User {
	if u == nil {
		return nil
	}

	id := strings.ToLower(u.Username)

	return &monitoring.User{
		ID:           id,
		Username:     u.Username,
		PasswordHash: u.Password,
		Role:         mapRoleToMonitoring(u.Role),
		DisplayName:  u.DisplayName,
		Email:        u.Email,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

func mapRoleToRepository(role string) string {
	switch role {
	case monitoring.RoleAdmin:
		return RoleAdmin
	case monitoring.RoleOps:
		return RoleOps
	case "viewer":
		return RoleOps
	default:
		return role
	}
}

func mapRoleToMonitoring(role string) string {
	switch role {
	case RoleAdmin:
		return monitoring.RoleAdmin
	case RoleOps, "viewer":
		return monitoring.RoleOps
	default:
		return role
	}
}
