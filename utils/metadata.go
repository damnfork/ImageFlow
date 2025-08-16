package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ImageMetadata stores metadata information for images
type ImageMetadata struct {
	ID           string           `json:"id"`           // Image ID (without extension)
	UserID       string           `json:"user_id"`      // User ID who owns this image
	OriginalName string           `json:"originalName"` // Original filename
	UploadTime   time.Time        `json:"uploadTime"`   // Upload timestamp
	ExpiryTime   time.Time        `json:"expiryTime"`   // Expiry timestamp (if set)
	Format       string           `json:"format"`       // Original format
	Orientation  string           `json:"orientation"`  // Image orientation
	Tags         []string         `json:"tags"`         // Image tags for categorization
	Sizes        map[string]int64 `json:"sizes"`        // File sizes for different formats
	Paths        struct {
		Original string `json:"original"` // Path to original image
		WebP     string `json:"webp"`     // Path to WebP format
		AVIF     string `json:"avif"`     // Path to AVIF format
	} `json:"paths"`
}

// MetadataStore defines the interface for metadata storage operations
type MetadataStore interface {
	SaveMetadata(ctx context.Context, metadata *ImageMetadata) error
	GetMetadata(ctx context.Context, id string) (*ImageMetadata, error)
	GetUserMetadata(ctx context.Context, userID string) ([]*ImageMetadata, error)
	ListExpiredImages(ctx context.Context) ([]*ImageMetadata, error)
	DeleteMetadata(ctx context.Context, id string) error
	GetAllMetadata(ctx context.Context) ([]*ImageMetadata, error)
	// Verify user ownership of an image
	VerifyImageOwnership(ctx context.Context, imageID, userID string) error
}

// LocalMetadataStore implements metadata storage for local filesystem
type LocalMetadataStore struct {
	BasePath string
}

// NewLocalMetadataStore creates a new local metadata store
func NewLocalMetadataStore(basePath string) (*LocalMetadataStore, error) {
	// Create metadata directory
	metadataDir := filepath.Join(basePath, "metadata")
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create metadata directory: %v", err)
	}
	return &LocalMetadataStore{BasePath: basePath}, nil
}

// SaveMetadata saves image metadata to a local file
func (lms *LocalMetadataStore) SaveMetadata(ctx context.Context, metadata *ImageMetadata) error {
	metadataDir := filepath.Join(lms.BasePath, "metadata")
	metadataPath := filepath.Join(metadataDir, metadata.ID+".json")

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %v", err)
	}

	logger.Info("Metadata saved successfully",
		zap.String("image_id", metadata.ID),
		zap.String("path", metadataPath))
	return nil
}

// GetMetadata retrieves image metadata from a local file
func (lms *LocalMetadataStore) GetMetadata(ctx context.Context, id string) (*ImageMetadata, error) {
	metadataPath := filepath.Join(lms.BasePath, "metadata", id+".json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %v", err)
	}

	var metadata ImageMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %v", err)
	}

	return &metadata, nil
}

// ListExpiredImages lists all expired images
func (lms *LocalMetadataStore) ListExpiredImages(ctx context.Context) ([]*ImageMetadata, error) {
	metadataDir := filepath.Join(lms.BasePath, "metadata")
	files, err := os.ReadDir(metadataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata directory: %v", err)
	}

	var expiredImages []*ImageMetadata
	now := time.Now()

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		metadataPath := filepath.Join(metadataDir, file.Name())
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			logger.Error("Failed to read metadata file",
				zap.String("path", metadataPath),
				zap.Error(err))
			continue
		}

		var metadata ImageMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			logger.Error("Failed to unmarshal metadata",
				zap.String("path", metadataPath),
				zap.Error(err))
			continue
		}

		// Check if the image has expired
		if !metadata.ExpiryTime.IsZero() && metadata.ExpiryTime.Before(now) {
			expiredImages = append(expiredImages, &metadata)
		}
	}

	logger.Debug("Listed expired images",
		zap.Int("count", len(expiredImages)))
	return expiredImages, nil
}

// DeleteMetadata deletes image metadata
func (lms *LocalMetadataStore) DeleteMetadata(ctx context.Context, id string) error {
	metadataPath := filepath.Join(lms.BasePath, "metadata", id+".json")
	return os.Remove(metadataPath)
}

// GetUserMetadata retrieves all metadata for a specific user
func (lms *LocalMetadataStore) GetUserMetadata(ctx context.Context, userID string) ([]*ImageMetadata, error) {
	allMetadata, err := lms.GetAllMetadata(ctx)
	if err != nil {
		return nil, err
	}

	var userMetadata []*ImageMetadata
	for _, metadata := range allMetadata {
		if metadata.UserID == userID {
			userMetadata = append(userMetadata, metadata)
		}
	}

	logger.Info("Retrieved user metadata entries",
		zap.String("user_id", userID),
		zap.Int("count", len(userMetadata)),
		zap.String("storage", "local"))
	return userMetadata, nil
}

// VerifyImageOwnership verifies that a user owns an image
func (lms *LocalMetadataStore) VerifyImageOwnership(ctx context.Context, imageID, userID string) error {
	metadata, err := lms.GetMetadata(ctx, imageID)
	if err != nil {
		return fmt.Errorf("failed to get image metadata: %v", err)
	}

	if metadata.UserID != userID {
		return fmt.Errorf("user %s does not own image %s", userID, imageID)
	}

	return nil
}

// GetAllMetadata retrieves all image metadata from local storage
func (lms *LocalMetadataStore) GetAllMetadata(ctx context.Context) ([]*ImageMetadata, error) {
	var allMetadata []*ImageMetadata
	metadataDir := filepath.Join(lms.BasePath, "metadata")

	// Read all files in the metadata directory
	files, err := os.ReadDir(metadataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		// Extract ID from filename
		id := strings.TrimSuffix(file.Name(), ".json")

		// Get metadata for this ID
		metadata, err := lms.GetMetadata(ctx, id)
		if err != nil {
			logger.Warn("Failed to get metadata",
				zap.String("id", id),
				zap.Error(err))
			continue
		}

		allMetadata = append(allMetadata, metadata)
	}

	logger.Info("Retrieved all metadata entries",
		zap.Int("count", len(allMetadata)),
		zap.String("storage", "local"))
	return allMetadata, nil
}

// S3MetadataStore implements metadata storage for S3
type S3MetadataStore struct {
	client *S3Storage
	prefix string
	bucket string
}

// NewS3MetadataStore creates a new S3 metadata store
func NewS3MetadataStore(s3Storage *S3Storage, cfg *config.Config) *S3MetadataStore {
	return &S3MetadataStore{
		client: s3Storage,
		prefix: "metadata/",
		bucket: cfg.S3Bucket,
	}
}

// SaveMetadata saves image metadata to S3
func (sms *S3MetadataStore) SaveMetadata(ctx context.Context, metadata *ImageMetadata) error {
	key := sms.prefix + metadata.ID + ".json"

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}

	if err := sms.client.Store(ctx, key, data); err != nil {
		return fmt.Errorf("failed to store metadata in S3: %v", err)
	}

	logger.Info("Metadata saved to S3",
		zap.String("image_id", metadata.ID),
		zap.String("key", key))
	return nil
}

// GetMetadata retrieves image metadata from S3
func (sms *S3MetadataStore) GetMetadata(ctx context.Context, id string) (*ImageMetadata, error) {
	key := sms.prefix + id + ".json"

	data, err := sms.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata from S3: %v", err)
	}

	var metadata ImageMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %v", err)
	}

	return &metadata, nil
}

// ListExpiredImages lists all expired images in S3
func (sms *S3MetadataStore) ListExpiredImages(ctx context.Context) ([]*ImageMetadata, error) {
	paginator := s3.NewListObjectsV2Paginator(S3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(sms.bucket),
		Prefix: aws.String(sms.prefix),
	})

	var expiredImages []*ImageMetadata
	now := time.Now()

	// Iterate through all pages of results
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			logger.Error("Failed to list metadata objects from S3", zap.Error(err))
			return nil, fmt.Errorf("failed to list metadata objects from S3: %v", err)
		}

		// Process each object in the current page
		for _, obj := range page.Contents {
			if !strings.HasSuffix(*obj.Key, ".json") {
				continue
			}

			id := strings.TrimSuffix(strings.TrimPrefix(*obj.Key, sms.prefix), ".json")
			metadata, err := sms.GetMetadata(ctx, id)
			if err != nil {
				logger.Error("Failed to get metadata from S3",
					zap.String("id", id),
					zap.Error(err))
				continue
			}

			if !metadata.ExpiryTime.IsZero() && metadata.ExpiryTime.Before(now) {
				expiredImages = append(expiredImages, metadata)
			}
		}
	}

	if len(expiredImages) > 0 {
		logger.Info("Found expired images in S3",
			zap.Int("count", len(expiredImages)))
	}
	return expiredImages, nil
}

// DeleteMetadata deletes image metadata from S3
func (sms *S3MetadataStore) DeleteMetadata(ctx context.Context, id string) error {
	key := sms.prefix + id + ".json"
	return sms.client.Delete(ctx, key)
}

// GetUserMetadata retrieves all metadata for a specific user from S3
func (s3ms *S3MetadataStore) GetUserMetadata(ctx context.Context, userID string) ([]*ImageMetadata, error) {
	allMetadata, err := s3ms.GetAllMetadata(ctx)
	if err != nil {
		return nil, err
	}

	var userMetadata []*ImageMetadata
	for _, metadata := range allMetadata {
		if metadata.UserID == userID {
			userMetadata = append(userMetadata, metadata)
		}
	}

	logger.Info("Retrieved user metadata entries from S3",
		zap.String("user_id", userID),
		zap.Int("count", len(userMetadata)))
	return userMetadata, nil
}

// VerifyImageOwnership verifies that a user owns an image in S3
func (s3ms *S3MetadataStore) VerifyImageOwnership(ctx context.Context, imageID, userID string) error {
	metadata, err := s3ms.GetMetadata(ctx, imageID)
	if err != nil {
		return fmt.Errorf("failed to get image metadata: %v", err)
	}

	if metadata.UserID != userID {
		return fmt.Errorf("user %s does not own image %s", userID, imageID)
	}

	return nil
}

// GetAllMetadata retrieves all image metadata from S3
func (s3ms *S3MetadataStore) GetAllMetadata(ctx context.Context) ([]*ImageMetadata, error) {
	var allMetadata []*ImageMetadata
	metadataPrefix := "metadata/"

	s3Storage, ok := Storage.(*S3Storage)
	if !ok {
		return nil, fmt.Errorf("failed to get S3 storage instance")
	}

	objects, err := s3Storage.ListObjects(ctx, metadataPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list metadata objects: %v", err)
	}

	for _, obj := range objects {
		if !strings.HasSuffix(obj.Key, ".json") {
			continue
		}

		id := strings.TrimSuffix(filepath.Base(obj.Key), ".json")
		metadata, err := s3ms.GetMetadata(ctx, id)
		if err != nil {
			logger.Warn("Failed to get metadata from S3",
				zap.String("id", id),
				zap.Error(err))
			continue
		}

		allMetadata = append(allMetadata, metadata)
	}

	logger.Info("Retrieved all metadata entries from S3",
		zap.Int("count", len(allMetadata)))
	return allMetadata, nil
}

// Global metadata storage instance
var MetadataManager MetadataStore

// InitMetadataStore initializes the metadata storage
func InitMetadataStore(cfg *config.Config) error {
	if err := InitRedisClient(cfg); err != nil {
		logger.Warn("Failed to initialize Redis client, falling back to file-based storage",
			zap.Error(err))
	}

	if IsRedisMetadataStore() {
		MetadataManager = NewRedisMetadataStore()
		logger.Info("Redis metadata store initialized")

		if _, err := RedisClient.Get(context.Background(), RedisPrefix+"migration_completed").Result(); err == redis.Nil {
			logger.Info("Starting metadata migration to Redis")
			if err := MigrateMetadataToRedis(context.Background(), cfg); err != nil {
				logger.Warn("Failed to migrate metadata to Redis",
					zap.Error(err))
			} else {
				RedisClient.Set(context.Background(), RedisPrefix+"migration_completed", time.Now().Format(time.RFC3339), 0)
				logger.Info("Metadata migration to Redis completed successfully")
			}
		} else if err != nil {
			logger.Warn("Failed to check Redis migration status",
				zap.Error(err))
		} else {
			logger.Debug("Metadata migration to Redis already completed")
		}

		return nil
	}

	if cfg.StorageType == config.StorageTypeS3 {
		if S3Client == nil {
			return fmt.Errorf("S3 client not initialized")
		}

		s3Storage, ok := Storage.(*S3Storage)
		if !ok {
			return fmt.Errorf("failed to get S3 storage instance")
		}

		MetadataManager = NewS3MetadataStore(s3Storage, cfg)
		logger.Info("S3 metadata store initialized",
			zap.String("bucket", cfg.S3Bucket))
	} else {
		localPath := cfg.ImageBasePath
		if !filepath.IsAbs(localPath) {
			localPath = filepath.Join(".", localPath)
		}

		localStore, err := NewLocalMetadataStore(localPath)
		if err != nil {
			return fmt.Errorf("failed to create local metadata store: %v", err)
		}

		MetadataManager = localStore
		logger.Info("Local metadata store initialized",
			zap.String("path", localPath))
	}

	return nil
}
