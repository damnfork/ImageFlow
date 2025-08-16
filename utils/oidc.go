package utils

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

// OIDCProvider wraps OIDC functionality
type OIDCProvider struct {
	Provider    *oidc.Provider
	Config      oauth2.Config
	Verifier    *oidc.IDTokenVerifier
	JWTSignKey  []byte
	Initialized bool
}

// Claims represents JWT claims for our session tokens
type Claims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	jwt.RegisteredClaims
}

// OIDCUserInfo represents user information from OIDC provider
type OIDCUserInfo struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// Global OIDC provider instance
var OIDCClient *OIDCProvider

// InitOIDCProvider initializes the OIDC provider
func InitOIDCProvider(cfg *config.Config) error {
	if cfg.AuthType == config.AuthTypeAPIKey {
		logger.Info("API Key authentication mode - OIDC not initialized")
		return nil
	}

	if cfg.OIDCIssuer == "" {
		return fmt.Errorf("OIDC issuer URL is required")
	}
	if cfg.OIDCClientID == "" {
		return fmt.Errorf("OIDC client ID is required")
	}
	if cfg.OIDCClientSecret == "" {
		return fmt.Errorf("OIDC client secret is required")
	}
	if cfg.OIDCRedirectURL == "" {
		return fmt.Errorf("OIDC redirect URL is required")
	}
	if cfg.JWTSigningKey == "" {
		return fmt.Errorf("JWT signing key is required")
	}

	ctx := context.Background()

	// Create OIDC provider
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return fmt.Errorf("failed to create OIDC provider: %v", err)
	}

	// Configure OAuth2
	oauth2Config := oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  cfg.OIDCRedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.OIDCScopes,
	}

	// Create ID token verifier
	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.OIDCClientID,
	})

	OIDCClient = &OIDCProvider{
		Provider:    provider,
		Config:      oauth2Config,
		Verifier:    verifier,
		JWTSignKey:  []byte(cfg.JWTSigningKey),
		Initialized: true,
	}

	logger.Info("OIDC provider initialized successfully",
		zap.String("issuer", cfg.OIDCIssuer),
		zap.String("client_id", cfg.OIDCClientID),
		zap.String("redirect_url", cfg.OIDCRedirectURL),
		zap.Strings("scopes", cfg.OIDCScopes))

	return nil
}

// GetAuthURL generates the OIDC authorization URL
func (o *OIDCProvider) GetAuthURL(state string) string {
	if !o.Initialized {
		logger.Error("OIDC provider not initialized")
		return ""
	}
	return o.Config.AuthCodeURL(state)
}

// ExchangeCodeForToken exchanges authorization code for tokens
func (o *OIDCProvider) ExchangeCodeForToken(ctx context.Context, code string) (*oauth2.Token, error) {
	if !o.Initialized {
		return nil, fmt.Errorf("OIDC provider not initialized")
	}

	token, err := o.Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %v", err)
	}

	return token, nil
}

// ExtractUserInfo extracts user information from ID token
func (o *OIDCProvider) ExtractUserInfo(ctx context.Context, token *oauth2.Token) (*OIDCUserInfo, error) {
	if !o.Initialized {
		return nil, fmt.Errorf("OIDC provider not initialized")
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token field in oauth2 token")
	}

	idToken, err := o.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %v", err)
	}

	var userInfo OIDCUserInfo
	if err := idToken.Claims(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to extract claims: %v", err)
	}

	return &userInfo, nil
}

// GenerateJWT generates a JWT token for the user session
func (o *OIDCProvider) GenerateJWT(user *User) (string, error) {
	if !o.Initialized {
		return "", fmt.Errorf("OIDC provider not initialized")
	}

	// Set token expiration (24 hours)
	expirationTime := time.Now().Add(24 * time.Hour)

	claims := &Claims{
		UserID:   user.ID,
		Email:    user.Email,
		Name:     user.Name,
		Provider: user.Provider,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "ImageFlow",
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(o.JWTSignKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %v", err)
	}

	return tokenString, nil
}

// ValidateJWT validates a JWT token and returns user claims
func (o *OIDCProvider) ValidateJWT(tokenString string) (*Claims, error) {
	if !o.Initialized {
		return nil, fmt.Errorf("OIDC provider not initialized")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return o.JWTSignKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT token: %v", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid JWT token")
}

// CreateOrUpdateUser creates or updates user information
func CreateOrUpdateUser(ctx context.Context, userInfo *OIDCUserInfo, provider string) (*User, error) {
	if UserManager == nil {
		return nil, fmt.Errorf("user manager not initialized")
	}

	// Try to get existing user
	existingUser, err := UserManager.GetUser(ctx, userInfo.Sub)
	if err != nil && err.Error() != fmt.Sprintf("user not found: %s", userInfo.Sub) {
		return nil, fmt.Errorf("error checking existing user: %v", err)
	}

	user := &User{
		ID:       userInfo.Sub,
		Email:    userInfo.Email,
		Name:     userInfo.Name,
		Picture:  userInfo.Picture,
		Provider: provider,
	}

	if existingUser == nil {
		// Create new user
		if err := UserManager.CreateUser(ctx, user); err != nil {
			return nil, fmt.Errorf("failed to create user: %v", err)
		}
		logger.Info("New user created",
			zap.String("user_id", user.ID),
			zap.String("email", user.Email),
			zap.String("provider", provider))
	} else {
		// Update existing user
		user.CreatedAt = existingUser.CreatedAt
		user.IsActive = existingUser.IsActive
		if err := UserManager.UpdateUser(ctx, user); err != nil {
			return nil, fmt.Errorf("failed to update user: %v", err)
		}
		logger.Info("User updated",
			zap.String("user_id", user.ID),
			zap.String("email", user.Email))
	}

	// Update last login
	if err := UserManager.UpdateLastLogin(ctx, user.ID); err != nil {
		logger.Warn("Failed to update last login time",
			zap.String("user_id", user.ID),
			zap.Error(err))
	}

	return user, nil
}

// GetUserFromRequest extracts user information from HTTP request
func GetUserFromRequest(r *http.Request) (*User, error) {
	if OIDCClient == nil || !OIDCClient.Initialized {
		return nil, fmt.Errorf("OIDC not initialized")
	}

	// Get token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("no authorization header")
	}

	// Extract Bearer token
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	tokenString := authHeader[len(bearerPrefix):]
	claims, err := OIDCClient.ValidateJWT(tokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT token: %v", err)
	}

	// Get user from store
	user, err := UserManager.GetUser(r.Context(), claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %v", err)
	}

	if !user.IsActive {
		return nil, fmt.Errorf("user account is deactivated")
	}

	return user, nil
}
