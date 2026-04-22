package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordChangeRequired is returned when a user must change their password.
var ErrPasswordChangeRequired = errors.New("password change required")

// Role constants.
const (
	RoleAdmin = "admin"
	RoleOps   = "ops"
)

// MongoUserRepository implements UserRepository using MongoDB.
type MongoUserRepository struct {
	client     *mongo.Client
	collection *mongo.Collection
}

// NewMongoUserRepository creates a new MongoDB user repository.
func NewMongoUserRepository(client *mongo.Client, dbName, prefix string) (*MongoUserRepository, error) {
	if client == nil {
		return nil, fmt.Errorf("mongo client is required")
	}
	if dbName == "" {
		dbName = "healthops"
	}
	if prefix == "" {
		prefix = "healthops"
	}

	repo := &MongoUserRepository{
		client:     client,
		collection: client.Database(dbName).Collection(prefix + "_users"),
	}

	indexCtx, indexCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer indexCancel()

	if err := repo.ensureIndexes(indexCtx); err != nil {
		fmt.Printf("WARNING: MongoDB user index creation deferred: %v\n", err)
	}

	return repo, nil
}

// ensureIndexes creates necessary indexes for the users collection.
func (r *MongoUserRepository) ensureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "usernameKey", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
	if err != nil && !indexAlreadyExists(err) {
		return err
	}
	return nil
}

// indexAlreadyExists checks if an index already exists error.
func indexAlreadyExists(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Name == "IndexOptionsConflict" || cmdErr.Code == 85
	}
	return false
}

// FindByUsername finds a user by username.
func (r *MongoUserRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}

	var user User
	filter := bson.M{"usernameKey": normalizeUsernameKey(username)}
	err := r.collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	user.Password = ""
	return &user, nil
}

// List returns all users.
func (r *MongoUserRepository) List(ctx context.Context) ([]User, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	cursor, err := r.collection.Find(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer cursor.Close(ctx)

	var users []User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, fmt.Errorf("failed to decode users: %w", err)
	}

	for i := range users {
		users[i].Password = ""
	}

	return users, nil
}

// Create creates a new user with hashed password.
func (r *MongoUserRepository) Create(ctx context.Context, user *User) error {
	if user == nil {
		return fmt.Errorf("user is required")
	}
	if user.Username == "" {
		return fmt.Errorf("username is required")
	}
	if user.Password == "" {
		return fmt.Errorf("password is required")
	}
	if len(user.Password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if user.Role != RoleAdmin && user.Role != RoleOps {
		return fmt.Errorf("role must be 'admin' or 'ops'")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now
	user.UsernameKey = normalizeUsernameKey(user.Username)
	user.Password = string(hashedPassword)
	if !user.Enabled {
		user.Enabled = true
	}

	filter := bson.M{"usernameKey": user.UsernameKey}
	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to check for existing user: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("username already exists")
	}

	_, err = r.collection.InsertOne(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	user.Password = ""
	return nil
}

// Update updates an existing user.
func (r *MongoUserRepository) Update(ctx context.Context, username string, user *User) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if user == nil {
		return fmt.Errorf("user is required")
	}
	if user.Role != "" && user.Role != RoleAdmin && user.Role != RoleOps {
		return fmt.Errorf("role must be 'admin' or 'ops'")
	}

	filter := bson.M{"usernameKey": normalizeUsernameKey(username)}
	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to check for existing user: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("user not found")
	}

	update := bson.M{
		"$set": bson.M{
			"updatedAt": time.Now().UTC(),
		},
	}

	if user.Role != "" {
		update["$set"].(bson.M)["role"] = user.Role
	}
	if user.Email != "" {
		update["$set"].(bson.M)["email"] = user.Email
	}
	if user.DisplayName != "" {
		update["$set"].(bson.M)["displayName"] = user.DisplayName
	}
	if user.Password != "" {
		if len(user.Password) < 8 {
			return fmt.Errorf("password must be at least 8 characters")
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		update["$set"].(bson.M)["password"] = string(hashedPassword)
		update["$set"].(bson.M)["mustChangePassword"] = false
	}
	if user.Enabled {
		update["$set"].(bson.M)["enabled"] = true
	}

	_, err = r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	user.Password = ""
	return nil
}

// Delete removes a user by username.
func (r *MongoUserRepository) Delete(ctx context.Context, username string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}

	user, err := r.FindByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if user.Role == RoleAdmin {
		adminCount, err := r.collection.CountDocuments(ctx, bson.M{"role": RoleAdmin})
		if err != nil {
			return fmt.Errorf("failed to check admin count: %w", err)
		}
		if adminCount <= 1 {
			return fmt.Errorf("cannot delete the last admin user")
		}
	}

	_, err = r.collection.DeleteOne(ctx, bson.M{"usernameKey": normalizeUsernameKey(username)})
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// BootstrapAdmin creates or resets the admin user when explicitly requested.
func (r *MongoUserRepository) BootstrapAdmin(ctx context.Context, password, email string, forceReset bool) (bool, error) {
	if password == "" {
		return false, fmt.Errorf("admin bootstrap password is required")
	}
	if len(password) < 8 {
		return false, fmt.Errorf("admin bootstrap password must be at least 8 characters")
	}
	if email == "" {
		email = "admin@healthops.local"
	}

	_, err := r.FindByUsername(ctx, "admin")
	switch {
	case err == nil:
		if !forceReset {
			return false, nil
		}
		update := &User{
			Role:        RoleAdmin,
			DisplayName: "Administrator",
			Email:       email,
			Password:    password,
			Enabled:     true,
		}
		if err := r.Update(ctx, "admin", update); err != nil {
			return false, fmt.Errorf("reset admin user: %w", err)
		}
		return true, nil
	case strings.Contains(err.Error(), "user not found"):
		create := &User{
			Username:    "admin",
			Password:    password,
			Role:        RoleAdmin,
			DisplayName: "Administrator",
			Email:       email,
			Enabled:     true,
		}
		if err := r.Create(ctx, create); err != nil {
			return false, fmt.Errorf("create admin user: %w", err)
		}
		return true, nil
	default:
		return false, fmt.Errorf("lookup admin user: %w", err)
	}
}

// Authenticate validates username and password, returning the user if valid.
func (r *MongoUserRepository) Authenticate(ctx context.Context, username, password string) (*User, error) {
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	var user User
	filter := bson.M{"usernameKey": normalizeUsernameKey(username)}
	err := r.collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	if !user.Enabled {
		return nil, fmt.Errorf("user account is disabled")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if user.MustChangePassword {
		user.Password = ""
		return &user, ErrPasswordChangeRequired
	}

	user.Password = ""
	return &user, nil
}

// HashPassword creates a bcrypt hash of the password.
func (r *MongoUserRepository) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to generate password hash: %w", err)
	}
	return string(hash), nil
}

// Ping checks if MongoDB is reachable.
func (r *MongoUserRepository) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("mongo client is nil")
	}
	return r.client.Ping(ctx, nil)
}

func normalizeUsernameKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
