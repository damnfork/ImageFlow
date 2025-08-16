package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/errors"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Global configuration instance
var globalConfig *config.Config

// SetConfig sets the global configuration
func SetConfig(cfg *config.Config) {
	globalConfig = cfg
}

func DebugLog(format string, args ...interface{}) {
	if globalConfig != nil && globalConfig.DebugMode {
		logger.Debug(fmt.Sprintf(format, args...))
	}
}

// ImageInfo represents information about an image
type ImageInfo = utils.ImageInfo

// PaginatedResponse represents a paginated response with images
type PaginatedResponse struct {
	Success    bool        `json:"success"`    // Whether the request was successful
	Images     []ImageInfo `json:"images"`     // Images for current page
	Page       int         `json:"page"`       // Current page number
	Limit      int         `json:"limit"`      // Number of items per page
	TotalPages int         `json:"totalPages"` // Total number of pages
	Total      int         `json:"total"`      // Total number of images
}

// ListImagesHandler returns a handler for listing images
func ListImagesHandler(cfg *config.Config) http.HandlerFunc {
	// Set global config for debug logging
	SetConfig(cfg)

	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		var cacheHit bool
		defer func() {
			if cfg.DebugMode {
				duration := time.Since(startTime)
				logger.Debug("List API latency",
					zap.Duration("duration", duration),
					zap.Bool("cache_hit", cacheHit))
			}
		}()

		// Get user from context (set by RequireAuth middleware)
		user, ok := GetUserFromContext(r.Context())
		if !ok {
			errors.HandleError(w, errors.ErrUnauthorized, "Authentication required", nil)
			return
		}

		// Parse query parameters
		params := parseQueryParams(r)

		var allImages []ImageInfo

		// Try to get from Redis if enabled
		if !utils.IsRedisMetadataStore() {
			errors.HandleError(w, errors.ErrInternal, "Redis is required for metadata storage", nil)
			return
		}

		cacheKey := utils.CachedPageKey{
			Orientation: params.orientation,
			Format:      params.format,
			Tag:         params.tag,
			Page:        params.page,
			Limit:       params.limit,
		}

		if cache, err := utils.GetCachedPage(r.Context(), cacheKey); err == nil {
			cacheHit = true
			allImages = cache.Data
		} else {
			// Cache miss, get from Redis
			var err error
			allImages, err = listImagesFromRedis(r.Context(), params, user, cfg)
			if err != nil {
				logger.Error("Failed to list images from Redis", zap.Error(err))
				errors.HandleError(w, errors.ErrImageList, "Failed to retrieve image list", err)
				return
			}

			// Cache the results
			if err := utils.SetCachedPage(r.Context(), cacheKey, allImages); err != nil {
				if cfg.DebugMode {
					logger.Debug("Failed to cache page results", zap.Error(err))
				}
			}
		}

		// Calculate pagination values
		total := len(allImages)
		totalPages := int(math.Ceil(float64(total) / float64(params.limit)))

		// Ensure page is within valid range
		if params.page > totalPages && totalPages > 0 {
			params.page = totalPages
		}

		// Calculate start and end indices for the current page
		startIdx := (params.page - 1) * params.limit
		endIdx := startIdx + params.limit
		if endIdx > total {
			endIdx = total
		}

		// Extract the subset of images for the current page
		var pagedImages []ImageInfo
		if startIdx < total {
			pagedImages = allImages[startIdx:endIdx]
		} else {
			pagedImages = []ImageInfo{}
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := PaginatedResponse{
			Success:    true,
			Images:     pagedImages,
			Page:       params.page,
			Limit:      params.limit,
			TotalPages: totalPages,
			Total:      total,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			if cfg.DebugMode {
				logger.Debug("Error encoding JSON response", zap.Error(err))
			}
		}
	}
}

// Query parameters structure
type queryParams struct {
	orientation string
	format      string
	tag         string // Tag to filter by
	page        int
	limit       int
}

// validateAPIKey checks if the provided API key is valid
func validateAPIKey(w http.ResponseWriter, r *http.Request, configAPIKey string) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		errors.HandleError(w, errors.ErrUnauthorized, "Authorization header not provided", nil)
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		errors.HandleError(w, errors.ErrUnauthorized, "Invalid authorization format", nil)
		return false
	}

	apiKey := parts[1]
	if apiKey != configAPIKey {
		errors.HandleError(w, errors.ErrUnauthorized, "Invalid API key", nil)
		return false
	}

	return true
}

// parseQueryParams extracts and validates query parameters
func parseQueryParams(r *http.Request) queryParams {
	orientation := r.URL.Query().Get("orientation")
	format := r.URL.Query().Get("format")
	tag := r.URL.Query().Get("tag")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	// Default values
	if orientation == "" {
		orientation = "all" // all, landscape, portrait
	}
	if format == "" {
		format = "original" // original, webp, avif
	}
	// Tag can be empty, which means no tag filtering

	// Set default pagination values
	page := 1
	limit := 12 // Default items per page

	// Parse page number
	if pageStr != "" {
		pageVal, err := strconv.Atoi(pageStr)
		if err == nil && pageVal > 0 {
			page = pageVal
		}
	}

	// Parse limit
	if limitStr != "" {
		limitVal, err := strconv.Atoi(limitStr)
		if err == nil && limitVal > 0 && limitVal <= 50 { // Cap at 50 items per page
			limit = limitVal
		}
	}

	return queryParams{
		orientation: orientation,
		format:      format,
		tag:         tag,
		page:        page,
		limit:       limit,
	}
}

// listImagesFromRedis retrieves images from Redis with optimized queries
func listImagesFromRedis(ctx context.Context, params queryParams, user *utils.User, cfg *config.Config) ([]ImageInfo, error) {
	if !utils.IsRedisMetadataStore() {
		return nil, fmt.Errorf("redis is not enabled")
	}

	// For OIDC users, get user-specific images
	// For API key users, get all images (backward compatibility)
	var imageIDs []string
	var err error

	if cfg.AuthType == config.AuthTypeOIDC && user.ID != "api_key_user" {
		// Get user-specific images
		userImagesKey := utils.RedisPrefix + "user:" + user.ID + ":images"
		imageIDs, err = utils.RedisClient.ZRevRange(ctx, userImagesKey, 0, -1).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to get user images: %v", err)
		}

		// If tag filtering is requested, filter user's images by tag
		if params.tag != "" {
			taggedImageIDs, err := utils.RedisClient.SMembers(ctx, utils.RedisPrefix+"tag:"+params.tag).Result()
			if err != nil {
				return nil, fmt.Errorf("failed to get tagged images: %v", err)
			}

			// Find intersection of user images and tagged images
			taggedSet := make(map[string]bool)
			for _, id := range taggedImageIDs {
				taggedSet[id] = true
			}

			var filteredIDs []string
			for _, id := range imageIDs {
				if taggedSet[id] {
					filteredIDs = append(filteredIDs, id)
				}
			}
			imageIDs = filteredIDs
		}
	} else {
		// Legacy behavior for API key users
		if params.tag != "" {
			// Get images by tag
			imageIDs, err = utils.RedisClient.SMembers(ctx, utils.RedisPrefix+"tag:"+params.tag).Result()
		} else {
			// Get all image IDs from sorted set
			imageIDs, err = utils.RedisClient.ZRevRange(ctx, utils.RedisPrefix+"images", 0, -1).Result()
		}

		if err != nil {
			return nil, fmt.Errorf("failed to get image IDs: %v", err)
		}
	}

	if len(imageIDs) == 0 {
		return []ImageInfo{}, nil
	}

	// Pre-allocate slice with capacity
	images := make([]ImageInfo, 0, len(imageIDs))

	// Use pipeline to get metadata for all images
	pipe := utils.RedisClient.Pipeline()
	metadataCommands := make(map[string]*redis.MapStringStringCmd, len(imageIDs))

	for _, id := range imageIDs {
		metadataCommands[id] = pipe.HGetAll(ctx, utils.RedisPrefix+"metadata:"+id)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %v", err)
	}

	// Process results
	for _, id := range imageIDs {
		data, err := metadataCommands[id].Result()
		if err != nil || len(data) == 0 {
			continue
		}

		// Filter by orientation if specified
		if params.orientation != "all" && data["orientation"] != params.orientation {
			continue
		}

		// Parse paths from JSON
		var paths struct {
			Original string `json:"original"`
			WebP     string `json:"webp"`
			AVIF     string `json:"avif"`
		}
		if pathsStr := data["paths"]; pathsStr != "" {
			if err := json.Unmarshal([]byte(pathsStr), &paths); err != nil {
				logger.Warn("Failed to unmarshal paths",
					zap.String("image_id", id),
					zap.Error(err))
			}
		}

		// Create image info
		imageInfo := ImageInfo{
			ID:          id,
			FileName:    data["originalName"],
			Orientation: data["orientation"],
			Format:      data["format"],
			StorageType: string(cfg.StorageType),
			URLs:        make(map[string]string, 3), // Pre-allocate with capacity
		}

		// Parse tags
		if tags := data["tags"]; tags != "" {
			imageInfo.Tags = strings.Split(tags, ",")
		}

		// Get base URL for image access
		baseURL := cfg.GetBaseURL()

		// Construct URLs based on paths
		isGIF := data["format"] == "gif"

		if isGIF {
			gifPath := filepath.Join("gif", id+".gif")
			gifURL := fmt.Sprintf("%s/%s", baseURL, strings.ReplaceAll(gifPath, "\\", "/"))
			imageInfo.URLs["original"] = gifURL
			imageInfo.URLs["webp"] = gifURL
			imageInfo.URLs["avif"] = gifURL
		} else {
			// Use stored paths if available
			if paths.Original != "" {
				imageInfo.URLs["original"] = fmt.Sprintf("%s/%s", baseURL, strings.ReplaceAll(paths.Original, "\\", "/"))
			} else {
				originalPath := filepath.Join("original", data["orientation"], id+"."+data["format"])
				imageInfo.URLs["original"] = fmt.Sprintf("%s/%s", baseURL, strings.ReplaceAll(originalPath, "\\", "/"))
			}

			if paths.WebP != "" {
				imageInfo.URLs["webp"] = fmt.Sprintf("%s/%s", baseURL, strings.ReplaceAll(paths.WebP, "\\", "/"))
			} else {
				webpPath := filepath.Join(data["orientation"], "webp", id+".webp")
				imageInfo.URLs["webp"] = fmt.Sprintf("%s/%s", baseURL, strings.ReplaceAll(webpPath, "\\", "/"))
			}

			if paths.AVIF != "" {
				imageInfo.URLs["avif"] = fmt.Sprintf("%s/%s", baseURL, strings.ReplaceAll(paths.AVIF, "\\", "/"))
			} else {
				avifPath := filepath.Join(data["orientation"], "avif", id+".avif")
				imageInfo.URLs["avif"] = fmt.Sprintf("%s/%s", baseURL, strings.ReplaceAll(avifPath, "\\", "/"))
			}
		}

		// Set the requested format URL
		imageInfo.URL = imageInfo.URLs[params.format]

		// Update filename based on format
		if params.format != "original" {
			baseName := strings.TrimSuffix(imageInfo.FileName, filepath.Ext(imageInfo.FileName))
			imageInfo.FileName = baseName + "." + params.format
		}

		// Get file size from Redis metadata (works for both local and S3 storage)
		if sizesStr := data["sizes"]; sizesStr != "" {
			var storedSizes map[string]int64
			if err := json.Unmarshal([]byte(sizesStr), &storedSizes); err == nil {
				if size, exists := storedSizes[params.format]; exists && size > 0 {
					imageInfo.Size = size
				}
			}
		}

		// Fallback: try the legacy size field for backward compatibility
		if imageInfo.Size == 0 {
			if sizeStr := data["size"]; sizeStr != "" {
				if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
					imageInfo.Size = size
				}
			}
		}

		images = append(images, imageInfo)
	}

	// Sort by filename in descending order
	sort.Slice(images, func(i, j int) bool {
		return images[i].FileName > images[j].FileName
	})

	return images, nil
}
