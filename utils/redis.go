package utils

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	// RedisClient is the global Redis client instance
	RedisClient *redis.Client
	// RedisPrefix is the prefix for all Redis keys
	RedisPrefix string
	// PageCacheExpiration is the expiration time for page cache
	PageCacheExpiration = 5 * time.Minute
	// PageCache mutex
	pageCacheMutex sync.RWMutex
	// Current metadata store type
	currentMetadataStoreType config.MetadataStoreType
)

// IsRedisMetadataStore checks if Redis is being used as the metadata store
func IsRedisMetadataStore() bool {
	return currentMetadataStoreType == config.MetadataStoreTypeRedis && RedisClient != nil
}

// ImageInfo represents information about an image
type ImageInfo struct {
	ID          string            `json:"id"`          // Filename without extension
	FileName    string            `json:"filename"`    // Full filename with extension
	URL         string            `json:"url"`         // URL to access the image
	URLs        map[string]string `json:"urls"`        // URLs for all available formats
	Orientation string            `json:"orientation"` // landscape or portrait
	Format      string            `json:"format"`      // original, webp, avif
	Size        int64             `json:"size"`        // File size in bytes
	Path        string            `json:"path"`        // Path relative to storage root
	StorageType string            `json:"storageType"` // "local" or "s3"
	Tags        []string          `json:"tags"`        // Image tags for categorization
}

// CachedPageKey represents a unique key for cached page results
type CachedPageKey struct {
	Orientation string `json:"orientation"`
	Format      string `json:"format"`
	Tag         string `json:"tag"`
	Page        int    `json:"page"`
	Limit       int    `json:"limit"`
}

// PageCache represents cached page data
type PageCache struct {
	Data      []ImageInfo `json:"data"`
	ExpiresAt time.Time   `json:"expires_at"`
}

// String returns a string representation of CachedPageKey
func (k CachedPageKey) String() string {
	return fmt.Sprintf("%s:%s:%s:%d:%d", k.Orientation, k.Format, k.Tag, k.Page, k.Limit)
}

// getCachedPage retrieves cached page data if available
func getCachedPage(ctx context.Context, key CachedPageKey) (*PageCache, error) {
	pageCacheMutex.RLock()
	defer pageCacheMutex.RUnlock()

	if !IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis not enabled")
	}

	cacheKey := RedisPrefix + "page_cache:" + key.String()
	data, err := RedisClient.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var cache PageCache
		if err := json.Unmarshal(data, &cache); err == nil {
			if time.Now().Before(cache.ExpiresAt) {
				return &cache, nil
			}
		}
	}
	return nil, fmt.Errorf("cache miss")
}

// setCachedPage stores page data in cache
func setCachedPage(ctx context.Context, key CachedPageKey, data []ImageInfo) error {
	pageCacheMutex.Lock()
	defer pageCacheMutex.Unlock()

	if !IsRedisMetadataStore() {
		return fmt.Errorf("redis not enabled")
	}

	cache := PageCache{
		Data:      data,
		ExpiresAt: time.Now().Add(PageCacheExpiration),
	}

	cacheData, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	cacheKey := RedisPrefix + "page_cache:" + key.String()
	return RedisClient.Set(ctx, cacheKey, cacheData, PageCacheExpiration).Err()
}

// ClearPageCache clears all page cache entries
func ClearPageCache(ctx context.Context) error {
	if !IsRedisMetadataStore() {
		return nil // Redis is not enabled, no need to clear cache
	}

	pattern := RedisPrefix + "page_cache:*"
	keys, err := RedisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return RedisClient.Del(ctx, keys...).Err()
	}
	return nil
}

// InitRedisClient initializes the Redis client
func InitRedisClient(cfg *config.Config) error {
	// Check if Redis is enabled as metadata store
	if cfg.MetadataStoreType != config.MetadataStoreTypeRedis {
		logger.Info("Redis is not configured as metadata store")
		RedisClient = nil
		currentMetadataStoreType = cfg.MetadataStoreType
		return nil
	}

	// Set Redis prefix based on storage type
	if cfg.StorageType == config.StorageTypeS3 {
		RedisPrefix = "imageflow:s3:"
	} else {
		RedisPrefix = "imageflow:local:"
	}

	// Clear page cache when storage type changes
	if err := ClearPageCache(context.Background()); err != nil {
		logger.Warn("Failed to clear page cache", zap.Error(err))
	}

	// Create Redis client
	redisOptions := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
	if cfg.RedisTLS {
		redisOptions.TLSConfig = &tls.Config{}
	}
	RedisClient = redis.NewClient(redisOptions)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := RedisClient.Ping(ctx).Err(); err != nil {
		logger.Error("Failed to connect to Redis", zap.Error(err))
		return err
	}

	logger.Info("Connected to Redis",
		zap.String("host", cfg.RedisHost),
		zap.String("port", cfg.RedisPort),
		zap.String("prefix", RedisPrefix))
	currentMetadataStoreType = cfg.MetadataStoreType
	return nil
}

// RedisMetadataStore implements metadata storage using Redis
// RedisMetadataStore is the structure for metadata operations using Redis.
type RedisMetadataStore struct {
	prefix string
}

// NewRedisMetadataStore creates a new Redis metadata store
func NewRedisMetadataStore() *RedisMetadataStore {
	return &RedisMetadataStore{
		prefix: RedisPrefix + "metadata:",
	}
}

// SaveMetadata saves image metadata to Redis with optimized structure
func (rms *RedisMetadataStore) SaveMetadata(ctx context.Context, metadata *ImageMetadata) error {
	if !IsRedisMetadataStore() {
		return fmt.Errorf("redis not enabled")
	}

	pipe := RedisClient.Pipeline()

	// Convert paths to JSON string
	pathsJSON, err := json.Marshal(metadata.Paths)
	if err != nil {
		return fmt.Errorf("failed to marshal paths: %v", err)
	}

	// Convert sizes to JSON string
	sizesJSON, err := json.Marshal(metadata.Sizes)
	if err != nil {
		return fmt.Errorf("failed to marshal sizes: %v", err)
	}

	// Store metadata in hash
	key := rms.prefix + metadata.ID
	pipe.HSet(ctx, key, map[string]interface{}{
		"id":           metadata.ID,
		"userID":       metadata.UserID,
		"originalName": metadata.OriginalName,
		"uploadTime":   metadata.UploadTime.Format(time.RFC3339),
		"expiryTime":   metadata.ExpiryTime.Format(time.RFC3339),
		"format":       metadata.Format,
		"orientation":  metadata.Orientation,
		"tags":         strings.Join(metadata.Tags, ","),
		"paths":        string(pathsJSON),
		"sizes":        string(sizesJSON),
	})

	// Add to user-specific image index
	if metadata.UserID != "" {
		userImagesKey := RedisPrefix + "user:" + metadata.UserID + ":images"
		pipe.ZAdd(ctx, userImagesKey, redis.Z{
			Score:  float64(metadata.UploadTime.Unix()),
			Member: metadata.ID,
		})
	}

	// Add to sorted set for pagination
	pipe.ZAdd(ctx, RedisPrefix+"images", redis.Z{
		Score:  float64(metadata.UploadTime.Unix()),
		Member: metadata.ID,
	})

	// Add to expiry index if expiry time is set
	if !metadata.ExpiryTime.IsZero() {
		expiryKey := RedisPrefix + "expiry"
		pipe.ZAdd(ctx, expiryKey, redis.Z{
			Score:  float64(metadata.ExpiryTime.Unix()),
			Member: metadata.ID,
		})
	}

	// Add tags
	if len(metadata.Tags) > 0 {
		for _, tag := range metadata.Tags {
			tagKey := RedisPrefix + "tag:" + tag
			pipe.SAdd(ctx, tagKey, metadata.ID)
		}

		// Add to all tags set
		allTagsKey := RedisPrefix + "all_tags"
		tagsInterface := make([]interface{}, len(metadata.Tags))
		for i, tag := range metadata.Tags {
			tagsInterface[i] = tag
		}
		pipe.SAdd(ctx, allTagsKey, tagsInterface...)
	}

	// Execute pipeline
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to save metadata to Redis: %v", err)
	}

	// Clear page cache when new data is added
	if err := ClearPageCache(ctx); err != nil {
		logger.Warn("Failed to clear page cache", zap.Error(err))
	}

	logger.Debug("Metadata saved to Redis",
		zap.String("id", metadata.ID),
		zap.Int("tags", len(metadata.Tags)))
	return nil
}

// GetMetadata retrieves image metadata from Redis with optimized structure
func (rms *RedisMetadataStore) GetMetadata(ctx context.Context, id string) (*ImageMetadata, error) {
	if !IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis not enabled")
	}

	key := rms.prefix + id
	data, err := RedisClient.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata from Redis: %v", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("metadata not found for ID: %s", id)
	}

	metadata := &ImageMetadata{
		ID:           data["id"],
		UserID:       data["userID"],
		OriginalName: data["originalName"],
		Format:       data["format"],
		Orientation:  data["orientation"],
	}

	// Parse times
	if uploadTime, err := time.Parse(time.RFC3339, data["uploadTime"]); err == nil {
		metadata.UploadTime = uploadTime
	}
	if expiryTime, err := time.Parse(time.RFC3339, data["expiryTime"]); err == nil {
		metadata.ExpiryTime = expiryTime
	}

	// Parse tags
	if tags := data["tags"]; tags != "" {
		metadata.Tags = strings.Split(tags, ",")
	}

	// Parse paths
	if paths := data["paths"]; paths != "" {
		json.Unmarshal([]byte(paths), &metadata.Paths)
	}

	// Parse sizes
	if sizes := data["sizes"]; sizes != "" {
		if metadata.Sizes == nil {
			metadata.Sizes = make(map[string]int64)
		}
		json.Unmarshal([]byte(sizes), &metadata.Sizes)
	}

	return metadata, nil
}

// ListExpiredImages lists all expired images
func (rms *RedisMetadataStore) ListExpiredImages(ctx context.Context) ([]*ImageMetadata, error) {
	now := time.Now()
	expiryKey := RedisPrefix + "expiry"

	// Get all expired image IDs (score <= current timestamp)
	expiredIDs, err := RedisClient.ZRangeByScore(ctx, expiryKey, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now.Unix()),
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to get expired image IDs: %v", err)
	}

	var expiredImages []*ImageMetadata
	for _, id := range expiredIDs {
		metadata, err := rms.GetMetadata(ctx, id)
		if err != nil {
			logger.Error("Failed to get metadata for expired image",
				zap.String("id", id),
				zap.Error(err))
			continue
		}

		expiredImages = append(expiredImages, metadata)
	}

	if len(expiredImages) > 0 {
		logger.Info("Found expired images",
			zap.Int("count", len(expiredImages)))
	}
	return expiredImages, nil
}

// DeleteMetadata deletes image metadata from Redis
func (rms *RedisMetadataStore) DeleteMetadata(ctx context.Context, id string) error {
	// Get metadata first to get tags
	metadata, err := rms.GetMetadata(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get metadata for deletion: %v", err)
	}

	// Remove from tag indexes
	for _, tag := range metadata.Tags {
		tagKey := RedisPrefix + "tag:" + tag
		if err := RedisClient.SRem(ctx, tagKey, id).Err(); err != nil {
			logger.Warn("Failed to remove from tag index",
				zap.String("tag", tag),
				zap.String("id", id),
				zap.Error(err))
		}
	}

	// Remove from expiry index
	expiryKey := RedisPrefix + "expiry"
	if err := RedisClient.ZRem(ctx, expiryKey, id).Err(); err != nil {
		logger.Warn("Failed to remove from expiry index",
			zap.String("id", id),
			zap.Error(err))
	}

	// Remove from main images index
	imagesKey := RedisPrefix + "images"
	if err := RedisClient.ZRem(ctx, imagesKey, id).Err(); err != nil {
		logger.Warn("Failed to remove from main images index",
			zap.String("id", id),
			zap.Error(err))
	}

	// Delete metadata
	key := rms.prefix + id
	if err := RedisClient.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete metadata from Redis: %v", err)
	}

	logger.Info("Metadata deleted from Redis",
		zap.String("id", id))
	return nil
}

// GetAllUniqueTags retrieves all unique tags from Redis
func GetAllUniqueTags(ctx context.Context) ([]string, error) {
	if !IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis is not enabled")
	}

	allTagsKey := RedisPrefix + "all_tags"
	tags, err := RedisClient.SMembers(ctx, allTagsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get tags from Redis: %v", err)
	}

	return tags, nil
}

// GetImagesByTag retrieves all image IDs with a specific tag
func GetImagesByTag(ctx context.Context, tag string) ([]string, error) {
	if !IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis is not enabled")
	}

	tagKey := RedisPrefix + "tag:" + tag
	imageIDs, err := RedisClient.SMembers(ctx, tagKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get images by tag from Redis: %v", err)
	}

	return imageIDs, nil
}

// GetImagesByMultipleTags retrieves image IDs that have ALL specified tags (AND logic)
func GetImagesByMultipleTags(ctx context.Context, tags []string) ([]string, error) {
	if !IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis is not enabled")
	}

	if len(tags) == 0 {
		return []string{}, nil
	}

	if len(tags) == 1 {
		// Single tag, use existing function
		return GetImagesByTag(ctx, tags[0])
	}

	// Multiple tags - use Redis SET intersection
	tagKeys := make([]string, len(tags))
	for i, tag := range tags {
		tagKeys[i] = RedisPrefix + "tag:" + tag
	}

	imageIDs, err := RedisClient.SInter(ctx, tagKeys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get images by multiple tags from Redis: %v", err)
	}

	logger.Debug("Retrieved images with multiple tags",
		zap.Strings("tags", tags),
		zap.Int("count", len(imageIDs)))

	return imageIDs, nil
}

// GetAllImageIDs retrieves all image IDs from Redis metadata
func GetAllImageIDs(ctx context.Context) ([]string, error) {
	if !IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis is not enabled")
	}

	// Use SCAN to get all metadata keys
	pattern := RedisPrefix + "metadata:*"
	var allKeys []string
	var cursor uint64

	for {
		keys, newCursor, err := RedisClient.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan Redis keys: %v", err)
		}

		allKeys = append(allKeys, keys...)
		cursor = newCursor

		if cursor == 0 {
			break
		}
	}

	// Extract image IDs from metadata keys
	imageIDs := make([]string, 0, len(allKeys))
	metadataPrefix := RedisPrefix + "metadata:"

	for _, key := range allKeys {
		if strings.HasPrefix(key, metadataPrefix) {
			id := strings.TrimPrefix(key, metadataPrefix)
			imageIDs = append(imageIDs, id)
		}
	}

	logger.Debug("Retrieved all image IDs from Redis",
		zap.Int("count", len(imageIDs)))

	return imageIDs, nil
}

// MigrateMetadataToRedis migrates metadata from JSON files to Redis
func MigrateMetadataToRedis(ctx context.Context, cfg *config.Config) error {
	if !IsRedisMetadataStore() {
		return fmt.Errorf("redis not enabled")
	}

	logger.Info("Starting metadata migration to Redis",
		zap.String("storage_type", string(cfg.StorageType)))

	// Create Redis metadata store
	redisStore := NewRedisMetadataStore()
	return migrateLocalMetadataToRedis(ctx, redisStore, cfg)
}

// migrateLocalMetadataToRedis migrates local metadata to Redis
func migrateLocalMetadataToRedis(ctx context.Context, redisStore *RedisMetadataStore, cfg *config.Config) error {
	// Ensure path is absolute
	localPath := cfg.ImageBasePath
	if !filepath.IsAbs(localPath) {
		localPath = filepath.Join(".", localPath)
	}

	metadataDir := filepath.Join(localPath, "metadata")
	files, err := os.ReadDir(metadataDir)
	if err != nil {
		return fmt.Errorf("failed to read metadata directory: %v", err)
	}

	migratedCount := 0
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		// Extract ID from filename
		id := filepath.Base(file.Name())
		id = id[:len(id)-5] // Remove .json extension

		// Read metadata file
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

		// Save to Redis
		if err := redisStore.SaveMetadata(ctx, &metadata); err != nil {
			logger.Error("Failed to save metadata to Redis",
				zap.String("id", id),
				zap.Error(err))
			continue
		}

		migratedCount++
	}

	logger.Info("Completed local metadata migration to Redis",
		zap.Int("migrated_count", migratedCount))
	return nil
}

// GetAllMetadata retrieves all image metadata from Redis
func (rms *RedisMetadataStore) GetAllMetadata(ctx context.Context) ([]*ImageMetadata, error) {
	if !IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis not enabled")
	}

	// Get all keys matching the metadata prefix pattern
	pattern := rms.prefix + "*"
	keys, err := RedisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata keys from Redis: %v", err)
	}

	var allMetadata []*ImageMetadata
	for _, key := range keys {
		// Extract ID from key
		id := strings.TrimPrefix(key, rms.prefix)

		// Get metadata for this ID
		metadata, err := rms.GetMetadata(ctx, id)
		if err != nil {
			logger.Warn("Failed to get metadata",
				zap.String("id", id),
				zap.Error(err))
			continue
		}

		allMetadata = append(allMetadata, metadata)
	}

	logger.Info("Retrieved all metadata entries from Redis",
		zap.Int("count", len(allMetadata)))
	return allMetadata, nil
}

// GetCachedPage retrieves cached page data if available
func GetCachedPage(ctx context.Context, key CachedPageKey) (*PageCache, error) {
	return getCachedPage(ctx, key)
}

// SetCachedPage stores page data in cache
func SetCachedPage(ctx context.Context, key CachedPageKey, data []ImageInfo) error {
	return setCachedPage(ctx, key, data)
}

// GetUserMetadata retrieves all metadata for a specific user from Redis
func (rms *RedisMetadataStore) GetUserMetadata(ctx context.Context, userID string) ([]*ImageMetadata, error) {
	if !IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis not enabled")
	}

	// Get all image IDs for the user
	userImagesKey := RedisPrefix + "user:" + userID + ":images"
	imageIDs, err := RedisClient.ZRevRange(ctx, userImagesKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user images from Redis: %v", err)
	}

	var userMetadata []*ImageMetadata
	for _, imageID := range imageIDs {
		metadata, err := rms.GetMetadata(ctx, imageID)
		if err != nil {
			logger.Warn("Failed to get metadata for user image",
				zap.String("user_id", userID),
				zap.String("image_id", imageID),
				zap.Error(err))
			continue
		}

		userMetadata = append(userMetadata, metadata)
	}

	logger.Info("Retrieved user metadata entries from Redis",
		zap.String("user_id", userID),
		zap.Int("count", len(userMetadata)))
	return userMetadata, nil
}

// VerifyImageOwnership verifies that a user owns an image in Redis
func (rms *RedisMetadataStore) VerifyImageOwnership(ctx context.Context, imageID, userID string) error {
	if !IsRedisMetadataStore() {
		return fmt.Errorf("redis not enabled")
	}

	metadata, err := rms.GetMetadata(ctx, imageID)
	if err != nil {
		return fmt.Errorf("failed to get image metadata: %v", err)
	}

	if metadata.UserID != userID {
		return fmt.Errorf("user %s does not own image %s", userID, imageID)
	}

	return nil
}
