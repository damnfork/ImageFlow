package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/errors"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

// DeleteRequest represents the request body for deleting an image
type DeleteRequest struct {
	ID string `json:"id"` // Image ID (filename without extension)
}

// DeleteResponse represents the response after deleting an image
type DeleteResponse struct {
	Success bool   `json:"success"` // Whether the operation was successful
	Message string `json:"message"` // Description of the result
}

// DeleteImageHandler returns a handler for deleting images
func DeleteImageHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST method
		if r.Method != http.MethodPost {
			errors.HandleError(w, errors.ErrInvalidParam, "Method not allowed", nil)
			logger.Warn("Invalid request method",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path))
			return
		}

		// Get user from context (set by RequireAuth middleware)
		user, ok := GetUserFromContext(r.Context())
		if !ok {
			errors.HandleError(w, errors.ErrUnauthorized, "Authentication required", nil)
			return
		}

		// Parse the request body
		var req DeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errors.HandleError(w, errors.ErrInvalidParam, "Invalid request body", nil)
			logger.Warn("Failed to decode request body",
				zap.Error(err))
			return
		}

		// Check if ID is provided
		if req.ID == "" {
			errors.HandleError(w, errors.ErrInvalidParam, "Image ID is required", nil)
			logger.Warn("Missing image ID")
			return
		}

		logger.Info("Processing delete request",
			zap.String("image_id", req.ID),
			zap.String("user_id", user.ID),
			zap.String("storage_type", string(cfg.StorageType)))

		// Verify image ownership (except for API key users)
		if cfg.AuthType == config.AuthTypeOIDC && user.ID != "api_key_user" {
			if err := utils.MetadataManager.VerifyImageOwnership(r.Context(), req.ID, user.ID); err != nil {
				errors.HandleError(w, errors.ErrForbidden, "You don't have permission to delete this image", nil)
				logger.Warn("User attempted to delete image they don't own",
					zap.String("user_id", user.ID),
					zap.String("image_id", req.ID),
					zap.Error(err))
				return
			}
		}

		var success bool
		var message string

		// Delete based on storage type
		if cfg.StorageType == config.StorageTypeS3 {
			success, message = deleteS3Images(req.ID, cfg)
		} else {
			success, message = deleteLocalImages(req.ID, cfg.ImageBasePath)
		}

		// If deletion was successful, clean up Redis data
		if success && utils.IsRedisMetadataStore() {
			// Create Redis metadata store
			redisStore := utils.NewRedisMetadataStore()

			// Delete metadata from Redis
			if err := redisStore.DeleteMetadata(r.Context(), req.ID); err != nil {
				logger.Warn("Failed to delete Redis metadata",
					zap.String("image_id", req.ID),
					zap.Error(err))
			}

			// Remove from images sorted set
			if err := utils.RedisClient.ZRem(r.Context(), utils.RedisPrefix+"images", req.ID).Err(); err != nil {
				logger.Warn("Failed to remove from images set",
					zap.String("image_id", req.ID),
					zap.Error(err))
			}

			// Clear page cache
			if err := utils.ClearPageCache(r.Context()); err != nil {
				logger.Warn("Failed to clear page cache",
					zap.String("image_id", req.ID),
					zap.Error(err))
			}
		}

		// Prepare and send response
		resp := DeleteResponse{
			Success: success,
			Message: message,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("Failed to encode response",
				zap.String("image_id", req.ID),
				zap.Error(err))
			errors.HandleError(w, errors.ErrInternal, "Internal server error", nil)
			return
		}

		logger.Info("Delete operation completed",
			zap.String("image_id", req.ID),
			zap.Bool("success", success),
			zap.String("message", message))
	}
}

// deleteLocalImages deletes all formats of an image from local storage
func deleteLocalImages(id string, basePath string) (bool, string) {
	// Formats and orientations to check for image files
	formats := []string{"original", "webp", "avif"}
	orientations := []string{"landscape", "portrait"}

	deletedCount := 0
	errorCount := 0
	var lastError error

	// Find all matching image files and delete them
	for _, format := range formats {
		for _, orientation := range orientations {
			var path string
			if format == "original" {
				path = filepath.Join(basePath, format, orientation)
			} else {
				path = filepath.Join(basePath, orientation, format)
			}

			// Find matching files with glob pattern
			files, err := filepath.Glob(filepath.Join(path, id+".*"))
			if err != nil {
				logger.Error("Failed to find files",
					zap.String("image_id", id),
					zap.String("path", path),
					zap.Error(err))
				errorCount++
				lastError = err
				continue
			}

			// Delete each found file
			for _, file := range files {
				err := os.Remove(file)
				if err != nil {
					logger.Error("Failed to delete file",
						zap.String("file", file),
						zap.Error(err))
					errorCount++
					lastError = err
				} else {
					logger.Debug("Successfully deleted file",
						zap.String("file", file))
					deletedCount++
				}
			}
		}
	}

	// Check for GIF files
	gifPath := filepath.Join(basePath, "gif")
	gifFiles, err := filepath.Glob(filepath.Join(gifPath, id+".*"))
	if err != nil {
		logger.Error("Failed to find GIF files",
			zap.String("image_id", id),
			zap.String("path", gifPath),
			zap.Error(err))
		errorCount++
		lastError = err
	} else {
		// Delete each GIF file found
		for _, file := range gifFiles {
			err := os.Remove(file)
			if err != nil {
				logger.Error("Failed to delete GIF file",
					zap.String("file", file),
					zap.Error(err))
				errorCount++
				lastError = err
			} else {
				logger.Debug("Successfully deleted GIF file",
					zap.String("file", file))
				deletedCount++
			}
		}
	}

	// Determine operation result
	if errorCount > 0 {
		return false, fmt.Sprintf("Partial deletion failure: %d files deleted successfully, %d failed: %v",
			deletedCount, errorCount, lastError)
	}

	if deletedCount == 0 {
		return false, "No matching image files found"
	}

	return true, fmt.Sprintf("Successfully deleted %d images", deletedCount)
}

// deleteS3Images deletes all formats of an image from S3 storage
func deleteS3Images(id string, cfg *config.Config) (bool, string) {
	// Check if S3 is properly configured
	if !cfg.S3Enabled {
		return false, "S3 storage is not enabled"
	}

	if cfg.S3Bucket == "" {
		return false, "S3 bucket not configured"
	}

	// Check if S3 client is initialized
	if utils.S3Client == nil {
		return false, "S3 client not initialized"
	}

	// Formats and orientations to check
	formats := []string{"original", "webp", "avif"}
	orientations := []string{"landscape", "portrait"}

	// Build list of objects to delete
	var objectsToDelete []types.ObjectIdentifier
	var deletedPathsForLogging []string

	// Create context
	ctx := context.Background()

	// Find matching objects
	for _, format := range formats {
		for _, orientation := range orientations {
			var prefix string
			if format == "original" {
				prefix = fmt.Sprintf("%s/%s/%s", format, orientation, id)
			} else {
				prefix = fmt.Sprintf("%s/%s/%s", orientation, format, id)
			}

			// List objects matching prefix
			paginator := s3.NewListObjectsV2Paginator(utils.S3Client, &s3.ListObjectsV2Input{
				Bucket: aws.String(cfg.S3Bucket),
				Prefix: aws.String(prefix),
			})

			for paginator.HasMorePages() {
				output, err := paginator.NextPage(ctx)
				if err != nil {
					logger.Error("Failed to list S3 objects",
						zap.String("prefix", prefix),
						zap.Error(err))
					continue
				}

				for _, obj := range output.Contents {
					key := *obj.Key
					// Check if filename starts with ID
					baseName := filepath.Base(key)
					if strings.HasPrefix(baseName, id+".") {
						objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
							Key: aws.String(key),
						})
						deletedPathsForLogging = append(deletedPathsForLogging, key)
					}
				}
			}
		}
	}

	// Check for GIF files
	gifPrefix := fmt.Sprintf("gif/%s", id)

	// List GIF objects matching prefix
	gifPaginator := s3.NewListObjectsV2Paginator(utils.S3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(cfg.S3Bucket),
		Prefix: aws.String(gifPrefix),
	})

	for gifPaginator.HasMorePages() {
		output, err := gifPaginator.NextPage(ctx)
		if err != nil {
			logger.Error("Failed to list S3 GIF objects",
				zap.String("prefix", gifPrefix),
				zap.Error(err))
			continue
		}

		for _, obj := range output.Contents {
			key := *obj.Key
			// Check if filename starts with ID
			baseName := filepath.Base(key)
			if strings.HasPrefix(baseName, id+".") {
				objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
					Key: aws.String(key),
				})
				deletedPathsForLogging = append(deletedPathsForLogging, key)
			}
		}
	}

	// If no matching objects found
	if len(objectsToDelete) == 0 {
		return false, "No matching image files found"
	}

	// Delete objects in batch
	_, err := utils.S3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(cfg.S3Bucket),
		Delete: &types.Delete{
			Objects: objectsToDelete,
			Quiet:   aws.Bool(false),
		},
	})

	if err != nil {
		logger.Error("Failed to delete S3 objects",
			zap.String("image_id", id),
			zap.Error(err))
		return false, fmt.Sprintf("Deletion failed: %v", err)
	}

	// Log deleted files
	for _, path := range deletedPathsForLogging {
		logger.Debug("Successfully deleted file from S3",
			zap.String("path", path))
	}

	return true, fmt.Sprintf("Successfully deleted %d images", len(objectsToDelete))
}
