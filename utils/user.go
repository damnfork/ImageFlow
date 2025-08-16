package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// User represents a user in the system
type User struct {
	ID        string    `json:"id"`         // User ID from OIDC provider
	Email     string    `json:"email"`      // User email
	Name      string    `json:"name"`       // User display name
	Picture   string    `json:"picture"`    // User avatar URL
	Provider  string    `json:"provider"`   // OIDC provider (e.g., "google", "auth0")
	CreatedAt time.Time `json:"created_at"` // When the user was first created
	UpdatedAt time.Time `json:"updated_at"` // When the user info was last updated
	LastLogin time.Time `json:"last_login"` // When the user last logged in
	IsActive  bool      `json:"is_active"`  // Whether the user account is active
}

// UserStore defines the interface for user storage operations
type UserStore interface {
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, userID string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	UpdateLastLogin(ctx context.Context, userID string) error
	ListUsers(ctx context.Context) ([]*User, error)
	DeactivateUser(ctx context.Context, userID string) error
}

// RedisUserStore implements user storage using Redis
type RedisUserStore struct {
	client     *redis.Client
	keyPrefix  string
	expiration time.Duration
}

// NewRedisUserStore creates a new Redis user store
func NewRedisUserStore() *RedisUserStore {
	if RedisClient == nil {
		logger.Fatal("Redis client not initialized")
		return nil
	}

	return &RedisUserStore{
		client:     RedisClient,
		keyPrefix:  RedisPrefix + "user:",
		expiration: 0, // Users don't expire
	}
}

// userKey returns the Redis key for a user
func (r *RedisUserStore) userKey(userID string) string {
	return r.keyPrefix + userID
}

// CreateUser creates a new user in Redis
func (r *RedisUserStore) CreateUser(ctx context.Context, user *User) error {
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	user.IsActive = true

	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %v", err)
	}

	key := r.userKey(user.ID)
	if err := r.client.Set(ctx, key, data, r.expiration).Err(); err != nil {
		return fmt.Errorf("failed to create user in Redis: %v", err)
	}

	// Add to user list for listing operations
	if err := r.client.SAdd(ctx, r.keyPrefix+"list", user.ID).Err(); err != nil {
		logger.Warn("Failed to add user to list", zap.String("user_id", user.ID), zap.Error(err))
	}

	logger.Info("User created successfully",
		zap.String("user_id", user.ID),
		zap.String("email", user.Email),
		zap.String("provider", user.Provider))
	return nil
}

// GetUser retrieves a user from Redis
func (r *RedisUserStore) GetUser(ctx context.Context, userID string) (*User, error) {
	key := r.userKey(userID)
	data, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		return nil, fmt.Errorf("failed to get user from Redis: %v", err)
	}

	var user User
	if err := json.Unmarshal([]byte(data), &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %v", err)
	}

	return &user, nil
}

// UpdateUser updates a user in Redis
func (r *RedisUserStore) UpdateUser(ctx context.Context, user *User) error {
	// Get existing user to preserve creation time
	existingUser, err := r.GetUser(ctx, user.ID)
	if err != nil {
		return err
	}

	user.CreatedAt = existingUser.CreatedAt
	user.UpdatedAt = time.Now()

	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %v", err)
	}

	key := r.userKey(user.ID)
	if err := r.client.Set(ctx, key, data, r.expiration).Err(); err != nil {
		return fmt.Errorf("failed to update user in Redis: %v", err)
	}

	logger.Info("User updated successfully",
		zap.String("user_id", user.ID),
		zap.String("email", user.Email))
	return nil
}

// UpdateLastLogin updates the last login time for a user
func (r *RedisUserStore) UpdateLastLogin(ctx context.Context, userID string) error {
	user, err := r.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	user.LastLogin = time.Now()
	user.UpdatedAt = time.Now()

	return r.UpdateUser(ctx, user)
}

// ListUsers returns all users in the system
func (r *RedisUserStore) ListUsers(ctx context.Context) ([]*User, error) {
	userIDs, err := r.client.SMembers(ctx, r.keyPrefix+"list").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user list from Redis: %v", err)
	}

	var users []*User
	for _, userID := range userIDs {
		user, err := r.GetUser(ctx, userID)
		if err != nil {
			logger.Warn("Failed to get user from list",
				zap.String("user_id", userID),
				zap.Error(err))
			continue
		}
		users = append(users, user)
	}

	logger.Info("Retrieved user list",
		zap.Int("count", len(users)))
	return users, nil
}

// DeactivateUser deactivates a user account
func (r *RedisUserStore) DeactivateUser(ctx context.Context, userID string) error {
	user, err := r.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	user.IsActive = false
	user.UpdatedAt = time.Now()

	if err := r.UpdateUser(ctx, user); err != nil {
		return err
	}

	logger.Info("User deactivated",
		zap.String("user_id", userID),
		zap.String("email", user.Email))
	return nil
}

// Global user store instance
var UserManager UserStore

// InitUserStore initializes the user storage
func InitUserStore(cfg *config.Config) error {
	if cfg.AuthType == config.AuthTypeAPIKey {
		logger.Info("API Key authentication mode - user store not initialized")
		return nil
	}

	if !IsRedisMetadataStore() {
		return fmt.Errorf("OIDC authentication requires Redis for user storage")
	}

	UserManager = NewRedisUserStore()
	logger.Info("Redis user store initialized")
	return nil
}
