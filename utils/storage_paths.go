package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"go.uber.org/zap"
)

// UserStoragePaths manages user-specific storage path generation
type UserStoragePaths struct {
	userID string
	cfg    *config.Config
}

// NewUserStoragePaths creates a new user storage path manager
func NewUserStoragePaths(userID string, cfg *config.Config) *UserStoragePaths {
	return &UserStoragePaths{
		userID: userID,
		cfg:    cfg,
	}
}

// GetOriginalPath returns the storage path for original images
func (usp *UserStoragePaths) GetOriginalPath(filename, orientation string) string {
	if usp.cfg.AuthType == config.AuthTypeAPIKey {
		// Legacy path for API Key users (no user isolation)
		return filepath.Join("original", orientation, filename)
	}

	// Multi-tenant path for OIDC users
	return filepath.Join("users", usp.userID, "original", orientation, filename)
}

// GetWebPPath returns the storage path for WebP images
func (usp *UserStoragePaths) GetWebPPath(filename, orientation string) string {
	if usp.cfg.AuthType == config.AuthTypeAPIKey {
		// Legacy path for API Key users
		return filepath.Join(orientation, "webp", filename+".webp")
	}

	// Multi-tenant path for OIDC users
	return filepath.Join("users", usp.userID, orientation, "webp", filename+".webp")
}

// GetAVIFPath returns the storage path for AVIF images
func (usp *UserStoragePaths) GetAVIFPath(filename, orientation string) string {
	if usp.cfg.AuthType == config.AuthTypeAPIKey {
		// Legacy path for API Key users
		return filepath.Join(orientation, "avif", filename+".avif")
	}

	// Multi-tenant path for OIDC users
	return filepath.Join("users", usp.userID, orientation, "avif", filename+".avif")
}

// GetGIFPath returns the storage path for GIF images
func (usp *UserStoragePaths) GetGIFPath(filename string) string {
	if usp.cfg.AuthType == config.AuthTypeAPIKey {
		// Legacy path for API Key users
		return filepath.Join("gif", filename)
	}

	// Multi-tenant path for OIDC users
	return filepath.Join("users", usp.userID, "gif", filename)
}

// GetUserDirectories returns all directories that need to be created for a user
func (usp *UserStoragePaths) GetUserDirectories() []string {
	if usp.cfg.AuthType == config.AuthTypeAPIKey {
		// Legacy directories (no user isolation)
		return []string{
			filepath.Join(usp.cfg.ImageBasePath, "original", "landscape"),
			filepath.Join(usp.cfg.ImageBasePath, "original", "portrait"),
			filepath.Join(usp.cfg.ImageBasePath, "landscape", "webp"),
			filepath.Join(usp.cfg.ImageBasePath, "landscape", "avif"),
			filepath.Join(usp.cfg.ImageBasePath, "portrait", "webp"),
			filepath.Join(usp.cfg.ImageBasePath, "portrait", "avif"),
			filepath.Join(usp.cfg.ImageBasePath, "gif"),
		}
	}

	// Multi-tenant directories for OIDC users
	userBasePath := filepath.Join(usp.cfg.ImageBasePath, "users", usp.userID)
	return []string{
		filepath.Join(userBasePath, "original", "landscape"),
		filepath.Join(userBasePath, "original", "portrait"),
		filepath.Join(userBasePath, "landscape", "webp"),
		filepath.Join(userBasePath, "landscape", "avif"),
		filepath.Join(userBasePath, "portrait", "webp"),
		filepath.Join(userBasePath, "portrait", "avif"),
		filepath.Join(userBasePath, "gif"),
	}
}

// EnsureUserDirectories creates necessary directories for a user
func (usp *UserStoragePaths) EnsureUserDirectories() error {
	dirs := usp.GetUserDirectories()

	for _, dir := range dirs {
		if err := ensureDirectory(dir); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	return nil
}

// GenerateStoragePaths generates all storage paths for an image
func (usp *UserStoragePaths) GenerateStoragePaths(imageID, format, orientation string) (original, webp, avif string) {
	if format == "gif" {
		original = usp.GetGIFPath(imageID + ".gif")
		webp = original // GIF uses same file for all formats
		avif = original
		return
	}

	// Generate extension based on format
	var ext string
	switch format {
	case "jpeg", "jpg":
		ext = ".jpg"
	case "png":
		ext = ".png"
	case "webp":
		ext = ".webp"
	default:
		ext = ".jpg" // Default fallback
	}

	original = usp.GetOriginalPath(imageID+ext, orientation)
	webp = usp.GetWebPPath(imageID, orientation)
	avif = usp.GetAVIFPath(imageID, orientation)
	return
}

// ensureDirectory creates a directory if it doesn't exist
func ensureDirectory(dir string) error {
	return createDirectoryIfNotExists(dir, 0755)
}

// createDirectoryIfNotExists creates a directory with the specified permissions if it doesn't exist
func createDirectoryIfNotExists(dir string, perm os.FileMode) error {
	if err := os.MkdirAll(dir, perm); err != nil {
		logger.Error("Failed to create directory",
			zap.String("dir", dir),
			zap.Error(err))
		return err
	}
	logger.Debug("Directory created or already exists",
		zap.String("dir", dir))
	return nil
}

// Legacy function for backward compatibility
func GenerateLegacyStoragePaths(imageID, format, orientation string) (original, webp, avif string) {
	if format == "gif" {
		original = filepath.Join("gif", imageID+".gif")
		webp = original
		avif = original
		return
	}

	var ext string
	switch format {
	case "jpeg", "jpg":
		ext = ".jpg"
	case "png":
		ext = ".png"
	case "webp":
		ext = ".webp"
	default:
		ext = ".jpg"
	}

	original = filepath.Join("original", orientation, imageID+ext)
	webp = filepath.Join(orientation, "webp", imageID+".webp")
	avif = filepath.Join(orientation, "avif", imageID+".avif")
	return
}
