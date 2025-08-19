package main

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/handlers"
	"github.com/Yuri-NagaSaki/ImageFlow/utils"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

// corsMiddleware adds CORS headers to all responses
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get allowed origins from environment variable or use a default value
		allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
		if allowedOrigins == "" {
			allowedOrigins = "*" // Default to allow all origins if not specified
		}

		// Handle multiple origins if specified
		origin := r.Header.Get("Origin")
		if origin != "" && allowedOrigins != "*" {
			// Check if the request origin is in the allowed origins list
			allowedList := strings.Split(allowedOrigins, ",")
			allowed := false
			for _, allowedOrigin := range allowedList {
				if strings.TrimSpace(allowedOrigin) == origin {
					allowed = true
					break
				}
			}

			if allowed {
				// Set the specific origin instead of the wildcard
				w.Header().Set("Access-Control-Allow-Origin", origin)
				// Allow credentials when specific origin is set
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			} else {
				// If origin is not allowed, still set the wildcard for public resources
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
		} else {
			// When using wildcard, we need to handle credentials properly
			if origin != "" {
				// Return the specific origin to support credentials
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			} else {
				// No origin header, use wildcard for simple requests
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
		}

		// Set other CORS headers
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

func main() {
	if err := logger.InitBasicLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize basic logger: %v\n", err)
		os.Exit(1)
	}

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		logger.Warn("Failed to load .env file", zap.Error(err))
	}

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Initialize logger with config
	if err := logger.InitLogger(cfg); err != nil {
		logger.Fatal("Failed to initialize logger", zap.Error(err))
	}
	defer logger.Log.Sync()

	// Initialize libvips for image processing
	utils.InitVips(cfg)
	logger.Info("Initialized libvips",
		zap.Int("worker_threads", cfg.WorkerThreads))

	// Initialize S3 client only when using S3 storage
	if cfg.StorageType == config.StorageTypeS3 {
		if err := utils.InitS3Client(cfg); err != nil {
			logger.Fatal("Failed to initialize S3 client", zap.Error(err))
		}
	}

	// Initialize storage provider
	if err := utils.InitStorage(cfg); err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}

	// Initialize metadata store
	if err := utils.InitMetadataStore(cfg); err != nil {
		logger.Fatal("Failed to initialize metadata store", zap.Error(err))
	}

	// Initialize OIDC provider
	if err := utils.InitOIDCProvider(cfg); err != nil {
		logger.Fatal("Failed to initialize OIDC provider", zap.Error(err))
	}

	// Initialize user store
	if err := utils.InitUserStore(cfg); err != nil {
		logger.Fatal("Failed to initialize user store", zap.Error(err))
	}

	// Ensure image directories exist
	ensureDirectories(cfg)

	// Initialize and start image cleaner
	utils.InitCleaner(cfg)
	logger.Info("Image cleaner started")

	// Configure MIME types
	configureMIMETypes()

	// Create routes

	// Authentication routes
	if cfg.AuthType == config.AuthTypeOIDC {
		// OIDC authentication routes (all under /api for consistency)
		http.HandleFunc("/api/auth/login", handlers.OIDCLoginHandler(cfg))
		http.HandleFunc("/auth/callback", handlers.OIDCCallbackHandler(cfg))        // Keep for backward compatibility
		http.HandleFunc("/api/auth/callback", handlers.OIDCCallbackAPIHandler(cfg)) // New API endpoint
		http.HandleFunc("/api/auth/logout", handlers.LogoutHandler(cfg))
		http.HandleFunc("/api/auth/profile", handlers.RequireAuth(cfg, handlers.UserProfileHandler(cfg)))
	} else {
		// Legacy API Key validation
		http.HandleFunc("/api/validate-api-key", handlers.ValidateAPIKey(cfg))
	}

	// Protected API routes (work with both auth types)
	http.HandleFunc("/api/upload", handlers.RequireAuth(cfg, handlers.UploadHandler(cfg)))
	http.HandleFunc("/api/images", handlers.RequireAuth(cfg, handlers.ListImagesHandler(cfg)))
	http.HandleFunc("/api/delete-image", handlers.RequireAuth(cfg, handlers.DeleteImageHandler(cfg)))
	http.HandleFunc("/api/config", handlers.RequireAuth(cfg, handlers.ConfigHandler(cfg)))
	http.HandleFunc("/api/tags", handlers.RequireAuth(cfg, handlers.TagsHandler(cfg)))
	http.HandleFunc("/api/debug/tags", handlers.RequireAuth(cfg, handlers.DebugTagsHandler(cfg)))

	// Add cleanup trigger endpoint
	http.HandleFunc("/api/trigger-cleanup", handlers.RequireAuth(cfg, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		utils.TriggerCleanup()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Cleanup process triggered",
		})
	}))

	// Use appropriate random image handler based on storage type
	if cfg.StorageType == config.StorageTypeS3 {
		http.HandleFunc("/api/random", handlers.RandomImageHandler(utils.S3Client, cfg))
	} else {
		http.HandleFunc("/api/random", handlers.LocalRandomImageHandler(cfg))
		// Serve local images
		if !filepath.IsAbs(cfg.ImageBasePath) {
			cfg.ImageBasePath = filepath.Join(".", cfg.ImageBasePath)
		}
		http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir(cfg.ImageBasePath))))
	}

	// Serve static files
	fs := http.FileServer(http.Dir("static"))

	// Next.js static assets
	http.Handle("/_next/", http.StripPrefix("/_next/", http.FileServer(http.Dir("static/_next"))))

	// Static assets
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Favicon files
	faviconServer := http.FileServer(http.Dir("favicon"))
	http.Handle("/favicon-16.png", faviconServer)
	http.Handle("/favicon-32.png", faviconServer)
	http.Handle("/favicon-48.png", faviconServer)
	http.Handle("/favicon.ico", faviconServer)
	http.Handle("/favicon.svg", faviconServer)

	// Text files
	http.HandleFunc("/index.txt", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/index.txt")
	})
	http.HandleFunc("/manage.txt", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/manage.txt")
	})

	// Serve upload and management pages
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.ServeFile(w, r, "static/index.html")
		case "/manage":
			http.ServeFile(w, r, "static/manage.html")
		default:
			filePath := filepath.Join("static", r.URL.Path)
			if !filepath.IsAbs(filePath) {
				http.NotFound(w, r)
				return
			}
			if _, err := os.Stat(filePath); err == nil {
				http.ServeFile(w, r, filePath)
			} else {
				http.NotFound(w, r)
			}
		}
	})

	// Create HTTP server
	server := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: corsMiddleware(http.DefaultServeMux),
	}

	// Set up graceful shutdown
	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		logger.Info("Starting server",
			zap.String("address", cfg.ServerAddr),
			zap.String("storage_type", string(cfg.StorageType)),
			zap.Bool("cors_enabled", true))

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server error", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	<-quit
	logger.Info("Server is shutting down...")

	// Give ongoing operations time to finish
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shut down the worker pool
	workerPool := utils.GetWorkerPool()
	if workerPool != nil {
		logger.Info("Shutting down worker pool...")
		workerPool.Shutdown()
	}

	// Stop the cleaner
	if utils.Cleaner != nil {
		logger.Info("Stopping image cleaner...")
		utils.Cleaner.Stop()
	}

	// Attempt to shut down the server gracefully
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	close(done)
	logger.Info("Server shutdown completed")
}

// configureMIMETypes registers common MIME types
func configureMIMETypes() {
	// Register common MIME types
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".ico", "image/x-icon")
	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".jpg", "image/jpeg")
	mime.AddExtensionType(".jpeg", "image/jpeg")
	mime.AddExtensionType(".webp", "image/webp")
	mime.AddExtensionType(".avif", "image/avif")
}

// ensureDirectories creates necessary directory structure for images
func ensureDirectories(cfg *config.Config) {
	dirs := []string{
		filepath.Join(cfg.ImageBasePath, "original", "landscape"),
		filepath.Join(cfg.ImageBasePath, "original", "portrait"),
		filepath.Join(cfg.ImageBasePath, "landscape", "webp"),
		filepath.Join(cfg.ImageBasePath, "landscape", "avif"),
		filepath.Join(cfg.ImageBasePath, "portrait", "webp"),
		filepath.Join(cfg.ImageBasePath, "portrait", "avif"),
		filepath.Join(cfg.ImageBasePath, "gif"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Error("Failed to create directory",
				zap.String("dir", dir),
				zap.Error(err))
		} else {
			logger.Info("Created directory",
				zap.String("dir", dir))
		}
	}
}
