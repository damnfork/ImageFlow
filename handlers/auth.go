package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/errors"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"go.uber.org/zap"
)

// AuthResponse represents the response for API key validation
type AuthResponse struct {
	Valid bool   `json:"valid"`           // Whether the API key is valid
	Error string `json:"error,omitempty"` // Error message if validation fails
}

// UserContextKey is the key for storing user in request context
type UserContextKey string

const (
	UserContextKeyValue UserContextKey = "user"
)

// ValidateAPIKey provides an endpoint to validate API keys
func ValidateAPIKey(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Get API key from request header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			errors.WriteError(w, errors.ErrInvalidAPIKey)
			return
		}

		// Extract Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			errors.WriteError(w, errors.ErrInvalidAPIKey)
			return
		}

		providedKey := parts[1]

		// Validate API key
		if providedKey == cfg.APIKey {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"valid":true}`))
			logger.Debug("API key validated successfully")
		} else {
			errors.WriteError(w, errors.ErrInvalidAPIKey)
			logger.Warn("API key validation failed",
				zap.String("provided_key", providedKey))
		}
	}
}

// RequireAuth middleware validates authentication based on the configured auth type
func RequireAuth(cfg *config.Config, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user *utils.User
		var err error

		switch cfg.AuthType {
		case config.AuthTypeAPIKey:
			// Legacy API Key authentication
			if err := validateAPIKeyAuth(cfg, r); err != nil {
				errors.WriteError(w, errors.ErrInvalidAPIKey)
				logger.Warn("API Key authentication failed",
					zap.String("path", r.URL.Path),
					zap.Error(err))
				return
			}
			// For API Key auth, we don't have a real user, so create a dummy user
			user = &utils.User{
				ID:    "api_key_user",
				Name:  "API Key User",
				Email: "api@imageflow.local",
			}

		case config.AuthTypeOIDC:
			// OIDC JWT authentication
			user, err = utils.GetUserFromRequest(r)
			if err != nil {
				errors.HandleError(w, errors.ErrUnauthorized, "Authentication failed", err.Error())
				logger.Warn("OIDC authentication failed",
					zap.String("path", r.URL.Path),
					zap.Error(err))
				return
			}

		default:
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("Invalid authentication type configured",
				zap.String("auth_type", string(cfg.AuthType)))
			return
		}

		// Add user to request context
		ctx := context.WithValue(r.Context(), UserContextKeyValue, user)
		r = r.WithContext(ctx)

		// Proceed to next handler
		next(w, r)
	}
}

// validateAPIKeyAuth validates API key authentication
func validateAPIKeyAuth(cfg *config.Config, r *http.Request) error {
	// Get API key from request header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return fmt.Errorf("missing authorization header")
	}

	// Extract Bearer token
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return fmt.Errorf("invalid authorization header format")
	}

	// Validate API key
	providedKey := parts[1]
	if providedKey != cfg.APIKey {
		return fmt.Errorf("invalid API key")
	}

	return nil
}

// RequireAPIKey is deprecated, use RequireAuth instead
// Kept for backward compatibility
func RequireAPIKey(cfg *config.Config, next http.HandlerFunc) http.HandlerFunc {
	logger.Warn("RequireAPIKey is deprecated, use RequireAuth instead")
	return RequireAuth(cfg, next)
}

// GetUserFromContext retrieves the user from request context
func GetUserFromContext(ctx context.Context) (*utils.User, bool) {
	user, ok := ctx.Value(UserContextKeyValue).(*utils.User)
	return user, ok
}
