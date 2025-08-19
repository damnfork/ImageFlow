package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/errors"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"go.uber.org/zap"
)

// LoginResponse represents the response after successful login
type LoginResponse struct {
	Token     string      `json:"token"`
	User      *utils.User `json:"user"`
	ExpiresAt int64       `json:"expires_at"`
}

// generateState generates a random state parameter for OIDC
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// OIDCLoginHandler initiates the OIDC login flow
func OIDCLoginHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.AuthType != config.AuthTypeOIDC {
			errors.HandleError(w, errors.ErrInvalidParam, "OIDC authentication not enabled", nil)
			logger.Warn("OIDC login attempted but OIDC auth is not enabled")
			return
		}

		if utils.OIDCClient == nil || !utils.OIDCClient.Initialized {
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("OIDC client not initialized")
			return
		}

		// Generate state parameter
		state, err := generateState()
		if err != nil {
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("Failed to generate state parameter", zap.Error(err))
			return
		}

		// Store state in session/cookie for validation (simplified approach)
		http.SetCookie(w, &http.Cookie{
			Name:     "oidc_state",
			Value:    state,
			Path:     "/",
			Expires:  time.Now().Add(10 * time.Minute),
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
		})

		// Get authorization URL
		authURL := utils.OIDCClient.GetAuthURL(state)
		if authURL == "" {
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("Failed to generate auth URL")
			return
		}

		// Return the auth URL for client-side redirect
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"auth_url": authURL,
			"state":    state,
		})

		logger.Info("OIDC login initiated",
			zap.String("state", state))
	}
}

// OIDCCallbackHandler handles the OIDC callback
func OIDCCallbackHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.AuthType != config.AuthTypeOIDC {
			http.Error(w, "OIDC auth not enabled", http.StatusBadRequest)
			return
		}

		if utils.OIDCClient == nil || !utils.OIDCClient.Initialized {
			http.Error(w, "OIDC not configured", http.StatusInternalServerError)
			logger.Error("OIDC client not initialized during callback")
			return
		}

		// Verify state parameter
		state := r.URL.Query().Get("state")
		stateCookie, err := r.Cookie("oidc_state")
		if err != nil || stateCookie.Value != state {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			logger.Warn("Invalid state parameter in OIDC callback",
				zap.String("provided_state", state),
				zap.Error(err))
			return
		}

		// Clear the state cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "oidc_state",
			Value:    "",
			Path:     "/",
			Expires:  time.Now().Add(-1 * time.Hour),
			HttpOnly: true,
		})

		// Get authorization code
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "No authorization code received", http.StatusBadRequest)
			logger.Warn("No authorization code in OIDC callback")
			return
		}

		// Exchange code for token
		token, err := utils.OIDCClient.ExchangeCodeForToken(r.Context(), code)
		if err != nil {
			http.Error(w, "Failed to exchange code for token", http.StatusInternalServerError)
			logger.Error("Failed to exchange authorization code",
				zap.String("code", code),
				zap.Error(err))
			return
		}

		// Extract user info from ID token
		userInfo, err := utils.OIDCClient.ExtractUserInfo(r.Context(), token)
		if err != nil {
			http.Error(w, "Failed to extract user info", http.StatusInternalServerError)
			logger.Error("Failed to extract user info from token", zap.Error(err))
			return
		}

		// Create or update user
		user, err := utils.CreateOrUpdateUser(r.Context(), userInfo, "oidc")
		if err != nil {
			http.Error(w, "Failed to create/update user", http.StatusInternalServerError)
			logger.Error("Failed to create or update user",
				zap.String("user_id", userInfo.Sub),
				zap.String("email", userInfo.Email),
				zap.Error(err))
			return
		}

		// Generate JWT for session
		sessionToken, err := utils.OIDCClient.GenerateJWT(user)
		if err != nil {
			http.Error(w, "Failed to generate session token", http.StatusInternalServerError)
			logger.Error("Failed to generate JWT token",
				zap.String("user_id", user.ID),
				zap.Error(err))
			return
		}

		// Calculate expiry time (24 hours from now)
		expiresAt := time.Now().Add(24 * time.Hour).Unix()

		// Return success response
		response := LoginResponse{
			Token:     sessionToken,
			User:      user,
			ExpiresAt: expiresAt,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode login response", zap.Error(err))
			return
		}

		logger.Info("User logged in successfully via OIDC",
			zap.String("user_id", user.ID),
			zap.String("email", user.Email),
			zap.String("provider", user.Provider))
	}
}

// OIDCCallbackRequest represents the callback request from frontend
type OIDCCallbackRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// OIDCCallbackAPIHandler handles OIDC callback via POST API from frontend
func OIDCCallbackAPIHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if cfg.AuthType != config.AuthTypeOIDC {
			errors.HandleError(w, errors.ErrInvalidParam, "OIDC authentication not enabled", nil)
			logger.Warn("OIDC callback attempted but OIDC auth is not enabled")
			return
		}

		if utils.OIDCClient == nil || !utils.OIDCClient.Initialized {
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("OIDC client not initialized during callback")
			return
		}

		// Parse JSON request body
		var callbackReq OIDCCallbackRequest
		if err := json.NewDecoder(r.Body).Decode(&callbackReq); err != nil {
			errors.HandleError(w, errors.ErrInvalidParam, "Invalid request body", err.Error())
			logger.Warn("Invalid callback request body", zap.Error(err))
			return
		}

		if callbackReq.Code == "" {
			errors.HandleError(w, errors.ErrInvalidParam, "Authorization code is required", nil)
			logger.Warn("No authorization code in OIDC callback request")
			return
		}

		if callbackReq.State == "" {
			errors.HandleError(w, errors.ErrInvalidParam, "State parameter is required", nil)
			logger.Warn("No state parameter in OIDC callback request")
			return
		}

		// Verify state parameter against cookie (same as GET callback for consistency)
		stateCookie, err := r.Cookie("oidc_state")
		if err != nil {
			errors.HandleError(w, errors.ErrInvalidParam, "Missing state cookie", err.Error())
			logger.Warn("Missing OIDC state cookie in API callback",
				zap.String("provided_state", callbackReq.State),
				zap.Error(err))
			return
		}

		if stateCookie.Value != callbackReq.State {
			errors.HandleError(w, errors.ErrInvalidParam, "Invalid state parameter", nil)
			logger.Warn("Invalid state parameter in OIDC API callback",
				zap.String("provided_state", callbackReq.State),
				zap.String("expected_state", stateCookie.Value))
			return
		}

		// Clear the state cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "oidc_state",
			Value:    "",
			Path:     "/",
			Expires:  time.Now().Add(-1 * time.Hour),
			HttpOnly: true,
		})

		// Exchange code for token
		token, err := utils.OIDCClient.ExchangeCodeForToken(r.Context(), callbackReq.Code)
		if err != nil {
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("Failed to exchange authorization code",
				zap.String("code", callbackReq.Code),
				zap.Error(err))
			return
		}

		// Extract user info from ID token
		userInfo, err := utils.OIDCClient.ExtractUserInfo(r.Context(), token)
		if err != nil {
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("Failed to extract user info from token", zap.Error(err))
			return
		}

		// Create or update user
		user, err := utils.CreateOrUpdateUser(r.Context(), userInfo, "oidc")
		if err != nil {
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("Failed to create or update user",
				zap.String("user_id", userInfo.Sub),
				zap.String("email", userInfo.Email),
				zap.Error(err))
			return
		}

		// Generate JWT for session
		sessionToken, err := utils.OIDCClient.GenerateJWT(user)
		if err != nil {
			errors.WriteError(w, errors.ErrServerError)
			logger.Error("Failed to generate JWT token",
				zap.String("user_id", user.ID),
				zap.Error(err))
			return
		}

		// Calculate expiry time (24 hours from now)
		expiresAt := time.Now().Add(24 * time.Hour).Unix()

		// Return success response
		response := LoginResponse{
			Token:     sessionToken,
			User:      user,
			ExpiresAt: expiresAt,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode login response", zap.Error(err))
			return
		}

		logger.Info("User logged in successfully via OIDC API",
			zap.String("user_id", user.ID),
			zap.String("email", user.Email),
			zap.String("provider", user.Provider))
	}
}

// LogoutHandler handles user logout
func LogoutHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For now, logout is client-side only (remove JWT token)
		// In the future, we could implement token blacklisting

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Logged out successfully",
		})

		logger.Info("User logged out")
	}
}

// UserProfileHandler returns current user profile
func UserProfileHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := utils.GetUserFromRequest(r)
		if err != nil {
			errors.HandleError(w, errors.ErrUnauthorized, "Authentication required", err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}
