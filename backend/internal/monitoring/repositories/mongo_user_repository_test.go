package repositories

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMongoUserRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(clientOpts().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
		return
	}
	defer client.Disconnect(ctx)

	repo, err := NewMongoUserRepository(client, "healthops_test", "test")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	_ = repo.collection.Drop(ctx)

	const bootstrapPassword = "AdminPass123!"

	t.Run("BootstrapAdminCreatesAdmin", func(t *testing.T) {
		changed, err := repo.BootstrapAdmin(ctx, bootstrapPassword, "admin@example.com", false)
		if err != nil {
			t.Fatalf("BootstrapAdmin failed: %v", err)
		}
		if !changed {
			t.Fatal("expected bootstrap to create admin user")
		}

		admin, err := repo.FindByUsername(ctx, "admin")
		if err != nil {
			t.Fatalf("Failed to find admin: %v", err)
		}
		if admin.Username != "admin" {
			t.Fatalf("expected username admin, got %s", admin.Username)
		}
		if admin.Role != RoleAdmin {
			t.Fatalf("expected role %s, got %s", RoleAdmin, admin.Role)
		}
		if admin.Email != "admin@example.com" {
			t.Fatalf("expected email admin@example.com, got %s", admin.Email)
		}
		if admin.Password != "" {
			t.Fatal("password should be cleared in returned user")
		}
	})

	t.Run("BootstrapAdminNoopWithoutReset", func(t *testing.T) {
		changed, err := repo.BootstrapAdmin(ctx, "AnotherPass123!", "changed@example.com", false)
		if err != nil {
			t.Fatalf("BootstrapAdmin failed: %v", err)
		}
		if changed {
			t.Fatal("expected bootstrap without reset to be a no-op when admin exists")
		}
	})

	t.Run("AuthenticateSuccessIsCaseInsensitive", func(t *testing.T) {
		user, err := repo.Authenticate(ctx, "ADMIN", bootstrapPassword)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if user.Username != "admin" {
			t.Fatalf("expected username admin, got %s", user.Username)
		}
		if user.Password != "" {
			t.Fatal("password should be cleared in authenticated user")
		}
	})

	t.Run("AuthenticateFailure", func(t *testing.T) {
		if _, err := repo.Authenticate(ctx, "admin", "wrongpassword"); err == nil {
			t.Fatal("expected error for wrong password")
		}
	})

	t.Run("CreateUser", func(t *testing.T) {
		newUser := &User{
			Username:    "OpsUser",
			Password:    "testpass123",
			Role:        RoleOps,
			DisplayName: "Ops User",
			Email:       "test@example.com",
			Enabled:     true,
		}

		if err := repo.Create(ctx, newUser); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if newUser.Password != "" {
			t.Fatal("password should be cleared after create")
		}

		user, err := repo.FindByUsername(ctx, "opsuser")
		if err != nil {
			t.Fatalf("Failed to find created user: %v", err)
		}
		if user.Email != "test@example.com" {
			t.Fatalf("expected email test@example.com, got %s", user.Email)
		}
		if user.DisplayName != "Ops User" {
			t.Fatalf("expected display name Ops User, got %s", user.DisplayName)
		}
	})

	t.Run("ListUsers", func(t *testing.T) {
		users, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(users) < 2 {
			t.Fatalf("expected at least 2 users, got %d", len(users))
		}
		for _, user := range users {
			if user.Password != "" {
				t.Fatal("password should be cleared in listed users")
			}
		}
	})

	t.Run("UpdateUser", func(t *testing.T) {
		updatedUser := &User{
			Email:       "updated@example.com",
			Role:        RoleAdmin,
			DisplayName: "Updated User",
		}

		if err := repo.Update(ctx, "opsuser", updatedUser); err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		user, err := repo.FindByUsername(ctx, "OpsUser")
		if err != nil {
			t.Fatalf("Failed to find updated user: %v", err)
		}
		if user.Email != "updated@example.com" {
			t.Fatalf("expected email updated@example.com, got %s", user.Email)
		}
		if user.Role != RoleAdmin {
			t.Fatalf("expected role %s, got %s", RoleAdmin, user.Role)
		}
		if user.DisplayName != "Updated User" {
			t.Fatalf("expected display name Updated User, got %s", user.DisplayName)
		}
	})

	t.Run("UpdatePassword", func(t *testing.T) {
		if err := repo.Update(ctx, "opsuser", &User{Password: "newpass123"}); err != nil {
			t.Fatalf("Update password failed: %v", err)
		}
		if _, err := repo.Authenticate(ctx, "opsuser", "newpass123"); err != nil {
			t.Fatalf("failed to authenticate with new password: %v", err)
		}
		if _, err := repo.Authenticate(ctx, "opsuser", "testpass123"); err == nil {
			t.Fatal("old password should not work after update")
		}
	})

	t.Run("BootstrapAdminReset", func(t *testing.T) {
		changed, err := repo.BootstrapAdmin(ctx, "ResetPass123!", "reset@example.com", true)
		if err != nil {
			t.Fatalf("BootstrapAdmin reset failed: %v", err)
		}
		if !changed {
			t.Fatal("expected bootstrap reset to update admin user")
		}
		if _, err := repo.Authenticate(ctx, "admin", "ResetPass123!"); err != nil {
			t.Fatalf("reset password should authenticate: %v", err)
		}
	})

	t.Run("DeleteUser", func(t *testing.T) {
		if err := repo.Delete(ctx, "opsuser"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if _, err := repo.FindByUsername(ctx, "opsuser"); err == nil {
			t.Fatal("expected error when finding deleted user")
		}
	})

	t.Run("CannotDeleteLastAdmin", func(t *testing.T) {
		if err := repo.Delete(ctx, "admin"); err == nil {
			t.Fatal("expected error when deleting last admin")
		}
	})

	t.Run("HashPassword", func(t *testing.T) {
		hash, err := repo.HashPassword("testpassword")
		if err != nil {
			t.Fatalf("HashPassword failed: %v", err)
		}
		if hash == "" || hash == "testpassword" {
			t.Fatal("expected bcrypt hash output")
		}
	})
}

func clientOpts() *options.ClientOptions {
	return options.Client().
		ApplyURI("mongodb://127.0.0.1:27017").
		SetServerSelectionTimeout(5 * time.Second)
}
