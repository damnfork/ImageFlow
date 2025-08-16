package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// StorageType defines the type of storage backend
type StorageType string

const (
	// StorageTypeLocal represents local file system storage
	StorageTypeLocal StorageType = "local"
	// StorageTypeS3 represents S3 compatible storage
	StorageTypeS3 StorageType = "s3"
	// StorageTypeDefault is the default storage type
	StorageTypeDefault = StorageTypeLocal
)

// MetadataStoreType defines the type of metadata storage backend
type MetadataStoreType string

const (
	// MetadataStoreTypeRedis represents Redis metadata storage
	MetadataStoreTypeRedis MetadataStoreType = "redis"
	// MetadataStoreTypeDefault is the default metadata storage type
	MetadataStoreTypeDefault = MetadataStoreTypeRedis
)

// AuthType defines the authentication method
type AuthType string

const (
	// AuthTypeAPIKey represents API key authentication (legacy)
	AuthTypeAPIKey AuthType = "api_key"
	// AuthTypeOIDC represents OIDC authentication
	AuthTypeOIDC AuthType = "oidc"
	// AuthTypeDefault is the default auth type
	AuthTypeDefault = AuthTypeOIDC
)

// Config stores the application configuration
type Config struct {
	// Server settings
	ServerAddr      string `json:"server_addr"`     // Server listen address
	ImageBasePath   string `json:"image_base_path"` // Base path for image storage
	AvifSupport     bool   `json:"avif_support"`    // Whether AVIF format is supported
	APIKey          string // API key for authentication (legacy, deprecated)
	MaxUploadCount  int    `json:"max_upload_count"` // Maximum number of images allowed in single upload
	ImageQuality    int    `json:"image_quality"`    // Image conversion quality (1-100)
	WorkerThreads   int    `json:"worker_threads"`   // Number of parallel worker threads
	Speed           int    `json:"speed"`            // Encoding speed (0-8, 0=slowest/highest quality)
	WorkerPoolSize  int    `json:"worker_pool_size"` // Size of worker pool for concurrent image processing
	DebugMode       bool   `json:"debug_mode"`       // Whether debug mode is enabled
	CleanupInterval int    `json:"cleanup_interval"` // Interval in minutes for cleaning expired images

	// Authentication settings
	AuthType AuthType `json:"auth_type"` // Type of authentication to use

	// OIDC settings
	OIDCIssuer       string   `json:"oidc_issuer"`       // OIDC provider issuer URL
	OIDCClientID     string   `json:"-"`                 // OIDC client ID
	OIDCClientSecret string   `json:"-"`                 // OIDC client secret
	OIDCRedirectURL  string   `json:"oidc_redirect_url"` // OIDC redirect URL
	OIDCScopes       []string `json:"oidc_scopes"`       // OIDC scopes to request
	JWTSigningKey    string   `json:"-"`                 // JWT signing key for session tokens

	// Storage settings
	StorageType  StorageType `json:"storage_type"`  // Type of storage backend to use
	CustomDomain string      `json:"custom_domain"` // Custom domain for S3 storage

	// Metadata storage settings
	MetadataStoreType MetadataStoreType `json:"metadata_store_type"` // Type of metadata storage to use

	// Redis settings
	RedisHost     string `json:"redis_host"` // Redis server host
	RedisPort     string `json:"redis_port"` // Redis server port
	RedisPassword string `json:"-"`          // Redis password
	RedisDB       int    `json:"redis_db"`   // Redis database number
	RedisTLS      bool   `json:"redis_tls"`  // Whether to use TLS for Redis connection

	// S3 settings
	S3Endpoint       string `json:"s3_endpoint"`         // S3 endpoint
	S3Region         string `json:"s3_region"`           // S3 region
	S3Bucket         string `json:"s3_bucket"`           // S3 bucket name
	S3AccessKey      string `json:"-"`                   // S3 access key
	S3SecretKey      string `json:"-"`                   // S3 secret key
	S3Enabled        bool   `json:"s3_enabled"`          // Whether S3 storage is enabled
	S3ForcePathStyle bool   `json:"s3_force_path_style"` // Use path style S3 URLs
}

// GetBaseURL returns the base URL for image access based on storage configuration
func (c *Config) GetBaseURL() string {
	if c.StorageType == StorageTypeS3 {
		if c.CustomDomain != "" {
			return strings.TrimSuffix(c.CustomDomain, "/")
		}
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(c.S3Endpoint, "/"), c.S3Bucket)
	}
	return "/images"
}

// ClientConfig represents the configuration exposed to clients
type ClientConfig struct {
	MaxUploadCount int  `json:"maxUploadCount"` // Maximum number of images allowed per upload
	ImageQuality   int  `json:"imageQuality"`   // Image conversion quality (1-100)
	Speed          int  `json:"speed"`          // Encoding speed (0-8, 0=slowest/highest quality)
	AvifSupport    bool `json:"avifSupport"`    // Whether AVIF format is supported
}

// GetClientConfig returns configuration that can be exposed to clients
func (c *Config) GetClientConfig() ClientConfig {
	return ClientConfig{
		MaxUploadCount: c.MaxUploadCount,
		ImageQuality:   c.ImageQuality,
		Speed:          c.Speed,
		AvifSupport:    c.AvifSupport,
	}
}

// Load loads configuration from environment variables and config file
func Load() (*Config, error) {
	// Default configuration
	cfg := &Config{
		ServerAddr:      "0.0.0.0:8686",
		ImageBasePath:   os.Getenv("LOCAL_STORAGE_PATH"),
		AvifSupport:     true,
		MaxUploadCount:  20,                 // Default max upload: 20 images
		ImageQuality:    75,                 // Default quality: 75
		WorkerThreads:   4,                  // Default workers: 4 threads
		Speed:           5,                  // Default speed: 5 (medium)
		WorkerPoolSize:  10,                 // Default worker pool size: 10 concurrent tasks
		StorageType:     StorageTypeDefault, // Default to local storage
		DebugMode:       false,              // Default debug mode off
		CleanupInterval: 1,                  // Default cleanup interval: 1 minute

		// Auth defaults
		AuthType: AuthTypeDefault, // Default to OIDC auth

		// OIDC defaults
		OIDCScopes: []string{"openid", "profile", "email"}, // Default OIDC scopes

		// Metadata store defaults
		MetadataStoreType: MetadataStoreTypeDefault,

		// Redis defaults
		RedisHost: "localhost",
		RedisPort: "6379",
		RedisDB:   0,
		RedisTLS:  false,

		// S3 defaults
		S3Region:         "us-east-1",
		S3ForcePathStyle: true,
		S3Enabled:        false,
	}

	// If LOCAL_STORAGE_PATH is not set, use default value
	if cfg.ImageBasePath == "" {
		cfg.ImageBasePath = "static/images"
	}

	// Ensure path is relative
	if !filepath.IsAbs(cfg.ImageBasePath) {
		cfg.ImageBasePath = filepath.Join(".", cfg.ImageBasePath)
	}

	// Try to load .env file, but don't require it
	_ = godotenv.Load()

	// Load environment variables
	cfg.loadEnvVars()

	// If config file exists, load additional configuration from file
	if _, err := os.Stat("config/config.json"); err == nil {
		file, err := os.Open("config/config.json")
		if err != nil {
			return nil, err
		}
		defer file.Close()

		decoder := json.NewDecoder(file)
		if err := decoder.Decode(cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// loadEnvVars loads configuration from environment variables
func (c *Config) loadEnvVars() {
	// Server settings
	if addr := os.Getenv("SERVER_ADDR"); addr != "" {
		c.ServerAddr = addr
	}
	c.APIKey = os.Getenv("API_KEY")

	// Debug mode
	if debug := os.Getenv("DEBUG_MODE"); debug != "" {
		c.DebugMode = debug == "true"
	}

	// Auth settings
	if authType := os.Getenv("AUTH_TYPE"); authType != "" {
		switch authType {
		case "api_key":
			c.AuthType = AuthTypeAPIKey
		case "oidc":
			c.AuthType = AuthTypeOIDC
		default:
			fmt.Printf("Warning: Invalid auth type specified (%s), using default OIDC\n", authType)
			c.AuthType = AuthTypeDefault
		}
	}

	// OIDC settings
	if issuer := os.Getenv("OIDC_ISSUER"); issuer != "" {
		c.OIDCIssuer = issuer
	}
	c.OIDCClientID = os.Getenv("OIDC_CLIENT_ID")
	c.OIDCClientSecret = os.Getenv("OIDC_CLIENT_SECRET")
	if redirectURL := os.Getenv("OIDC_REDIRECT_URL"); redirectURL != "" {
		c.OIDCRedirectURL = redirectURL
	}
	if scopes := os.Getenv("OIDC_SCOPES"); scopes != "" {
		c.OIDCScopes = strings.Split(scopes, ",")
		// Trim spaces
		for i, scope := range c.OIDCScopes {
			c.OIDCScopes[i] = strings.TrimSpace(scope)
		}
	}
	c.JWTSigningKey = os.Getenv("JWT_SIGNING_KEY")

	// Storage settings
	if storageType := os.Getenv("STORAGE_TYPE"); storageType != "" {
		switch storageType {
		case "local":
			c.StorageType = StorageTypeLocal
		case "s3":
			c.StorageType = StorageTypeS3
			// When storage type is S3, automatically enable S3
			c.S3Enabled = true
			fmt.Printf("Storage type set to S3, automatically enabling S3\n")
		default:
			fmt.Printf("Warning: Invalid storage type specified (%s), using local storage\n", storageType)
			c.StorageType = StorageTypeLocal
		}
	}
	if customDomain := os.Getenv("CUSTOM_DOMAIN"); customDomain != "" {
		c.CustomDomain = customDomain
	}

	// Parse integer environment variables
	envVarInt := map[string]*int{
		"MAX_UPLOAD_COUNT": &c.MaxUploadCount,
		"IMAGE_QUALITY":    &c.ImageQuality,
		"WORKER_THREADS":   &c.WorkerThreads,
		"SPEED":            &c.Speed,
		"WORKER_POOL_SIZE": &c.WorkerPoolSize,
		"REDIS_DB":         &c.RedisDB,
		"CLEANUP_INTERVAL": &c.CleanupInterval,
	}

	for envName, ptr := range envVarInt {
		if val := os.Getenv(envName); val != "" {
			if num, err := strconv.Atoi(val); err == nil {
				*ptr = num
			}
		}
	}

	// Ensure speed is within valid range (0-8)
	if c.Speed < 0 {
		c.Speed = 0
	} else if c.Speed > 8 {
		c.Speed = 8
	}

	// Redis settings
	if host := os.Getenv("REDIS_HOST"); host != "" {
		c.RedisHost = host
	}
	if port := os.Getenv("REDIS_PORT"); port != "" {
		c.RedisPort = port
	}
	c.RedisPassword = os.Getenv("REDIS_PASSWORD")

	// Metadata store settings
	if storeType := os.Getenv("METADATA_STORE_TYPE"); storeType != "" {
		switch storeType {
		case "redis":
			c.MetadataStoreType = MetadataStoreTypeRedis
		default:
			fmt.Printf("Warning: Invalid metadata store type specified (%s), using default\n", storeType)
			c.MetadataStoreType = MetadataStoreTypeDefault
		}
	}

	if tls := os.Getenv("REDIS_TLS_ENABLED"); tls != "" {
		c.RedisTLS = tls == "true"
	}

	// S3 settings
	if endpoint := os.Getenv("S3_ENDPOINT"); endpoint != "" {
		c.S3Endpoint = endpoint
	}
	if region := os.Getenv("S3_REGION"); region != "" {
		c.S3Region = region
	}
	if bucket := os.Getenv("S3_BUCKET"); bucket != "" {
		c.S3Bucket = bucket
	}
	c.S3AccessKey = os.Getenv("S3_ACCESS_KEY")
	c.S3SecretKey = os.Getenv("S3_SECRET_KEY")

	// Handle S3_ENABLED override
	if enabled := os.Getenv("S3_ENABLED"); enabled != "" {
		c.S3Enabled = enabled == "true"
		// If S3 is explicitly disabled but storage type is S3, force local storage
		if !c.S3Enabled && c.StorageType == StorageTypeS3 {
			c.StorageType = StorageTypeLocal
			fmt.Printf("Warning: S3 is disabled but storage type was set to S3, forcing local storage\n")
		}
	}

	if pathStyle := os.Getenv("S3_FORCE_PATH_STYLE"); pathStyle != "" {
		c.S3ForcePathStyle = pathStyle == "true"
	}
}

// IsValidStorageType checks if the storage type is valid
func (s StorageType) IsValidStorageType() bool {
	return s == StorageTypeLocal || s == StorageTypeS3
}
