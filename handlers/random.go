package handlers

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/errors"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

// RandomQueryParams holds all query parameters for random image API
type RandomQueryParams struct {
	Tags        []string // Multiple tags (comma-separated)
	ExcludeTags []string // Tags to exclude (comma-separated)
	Orientation string   // portrait, landscape, or both
	Format      string   // preferred format hint
}

// parseRandomQueryParams extracts and validates query parameters
func parseRandomQueryParams(r *http.Request) *RandomQueryParams {
	params := &RandomQueryParams{}

	// Parse tags - support both single tag and multiple tags
	if tagStr := r.URL.Query().Get("tag"); tagStr != "" {
		params.Tags = strings.Split(tagStr, ",")
		// Trim spaces from each tag
		for i, tag := range params.Tags {
			params.Tags[i] = strings.TrimSpace(tag)
		}
	}
	if tagsStr := r.URL.Query().Get("tags"); tagsStr != "" {
		tags := strings.Split(tagsStr, ",")
		for _, tag := range tags {
			if trimmed := strings.TrimSpace(tag); trimmed != "" {
				params.Tags = append(params.Tags, trimmed)
			}
		}
	}

	// Parse exclude tags
	if excludeStr := r.URL.Query().Get("exclude"); excludeStr != "" {
		params.ExcludeTags = strings.Split(excludeStr, ",")
		for i, tag := range params.ExcludeTags {
			params.ExcludeTags[i] = strings.TrimSpace(tag)
		}
	}

	// Parse orientation
	params.Orientation = strings.ToLower(r.URL.Query().Get("orientation"))
	if params.Orientation != "portrait" && params.Orientation != "landscape" {
		params.Orientation = "" // Will be auto-detected
	}

	// Parse format preference
	params.Format = strings.ToLower(r.URL.Query().Get("format"))

	return params
}

// matchesTags checks if an image matches the tag criteria
func matchesTags(imageTags []string, requiredTags []string, excludeTags []string) bool {
	// Convert to map for faster lookup
	imageTagMap := make(map[string]bool)
	for _, tag := range imageTags {
		imageTagMap[tag] = true
	}

	// Check if image has any excluded tags
	for _, excludeTag := range excludeTags {
		if imageTagMap[excludeTag] {
			return false
		}
	}

	// If no required tags specified, and no excluded tags matched, it's valid
	if len(requiredTags) == 0 {
		return true
	}

	// Check if image has ALL required tags (AND logic)
	for _, requiredTag := range requiredTags {
		if !imageTagMap[requiredTag] {
			return false
		}
	}

	return true
}

// Image format constants
const (
	FormatAVIF     = "avif"
	FormatWebP     = "webp"
	FormatOriginal = "original"
)

// detectBestFormat determines optimal image format based on Accept headers
func detectBestFormat(r *http.Request) string {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "image/avif") {
		return FormatAVIF
	}
	if strings.Contains(accept, "image/webp") {
		return FormatWebP
	}
	return FormatOriginal
}

// determineOrientation selects orientation based on device type and request parameters
func determineOrientation(r *http.Request, deviceType string) string {
	orientation := r.URL.Query().Get("orientation")
	if orientation == "" {
		if deviceType == utils.DeviceMobile {
			return "portrait"
		} else {
			return "landscape"
		}
	}
	return orientation
}

// getContentType returns the appropriate Content-Type based on format and filename
func getContentType(format string, filename string) string {
	if format == FormatAVIF {
		return "image/avif"
	}
	if format == FormatWebP {
		return "image/webp"
	}

	// Determine content type based on file extension
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

// setImageResponseHeaders sets standard HTTP headers for image responses
func setImageResponseHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Vary", "Accept, User-Agent")
}

// getFormattedImagePath constructs the path to an image with the given format
func getFormattedImagePath(format string, orientation string, filename string) string {
	switch format {
	case FormatAVIF:
		return fmt.Sprintf("%s/avif/%s.avif", orientation, filename)
	case FormatWebP:
		return fmt.Sprintf("%s/webp/%s.webp", orientation, filename)
	default:
		// Keep original format and path
		// Determine extension by checking if filename already has one
		extension := filepath.Ext(filename)
		if extension == "" {
			extension = ".jpg" // Default to jpg if no extension
		}
		return fmt.Sprintf("original/%s/%s%s", orientation, filename, extension)
	}
}

// RandomImageHandler serves random images from S3 storage
func RandomImageHandler(s3Client *s3.Client, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.S3Enabled {
			errors.HandleError(w, errors.ErrInternal, "S3 storage is not enabled", nil)
			return
		}

		// Parse query parameters
		params := parseRandomQueryParams(r)

		// Determine device type and orientation
		deviceType := utils.DetectDeviceType(r)
		orientation := determineOrientation(r, deviceType)

		// Override orientation if specified in params
		if params.Orientation != "" {
			orientation = params.Orientation
		}

		logger.Info("Processing random image request",
			zap.Strings("tags", params.Tags),
			zap.Strings("exclude_tags", params.ExcludeTags),
			zap.String("orientation", orientation),
			zap.String("device_type", deviceType))

		// Find matching images
		var matchingImages []string
		var err error

		// Use Redis for efficient filtering if available and tags are specified
		if (len(params.Tags) > 0 || len(params.ExcludeTags) > 0) && utils.IsRedisMetadataStore() {
			var candidateIDs []string

			if len(params.Tags) > 0 {
				// Get images that have ALL required tags
				candidateIDs, err = utils.GetImagesByMultipleTags(context.Background(), params.Tags)
				if err != nil {
					logger.Error("Failed to get images by tags from Redis", zap.Error(err))
					// Fall back to traditional method
				}
			} else {
				// Get all image IDs if only exclude filters are specified
				candidateIDs, err = utils.GetAllImageIDs(context.Background())
				if err != nil {
					logger.Error("Failed to get all image IDs from Redis", zap.Error(err))
				}
			}

			if err == nil && len(candidateIDs) > 0 {
				// Filter by metadata
				for _, id := range candidateIDs {
					metadata, metaErr := utils.MetadataManager.GetMetadata(context.Background(), id)
					if metaErr != nil {
						continue
					}

					// Check tag matching
					if !matchesTags(metadata.Tags, params.Tags, params.ExcludeTags) {
						continue
					}

					// Check orientation
					if metadata.Orientation == orientation {
						matchingImages = append(matchingImages, metadata.Paths.Original)
					}
				}

				logger.Info("Found matching images from Redis",
					zap.Int("count", len(matchingImages)))
			}
		}

		// Fall back to S3 listing if Redis didn't work or no results
		if len(matchingImages) == 0 {
			// Build prefix for orientation directory
			prefix := fmt.Sprintf("original/%s/", orientation)

			output, err := s3Client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
				Bucket: aws.String(cfg.S3Bucket),
				Prefix: aws.String(prefix),
			})

			if err != nil {
				logger.Error("Failed to list objects from S3", zap.Error(err))
				errors.HandleError(w, errors.ErrInternal, "Failed to list images", err)
				return
			}

			// Filter images based on criteria
			for _, obj := range output.Contents {
				if !utils.IsImageFile(*obj.Key) {
					continue
				}

				// Extract ID for metadata lookup
				fileBaseName := filepath.Base(*obj.Key)
				id := strings.TrimSuffix(fileBaseName, filepath.Ext(fileBaseName))

				// Get metadata for tag filtering
				if len(params.Tags) > 0 || len(params.ExcludeTags) > 0 {
					metadata, metaErr := utils.MetadataManager.GetMetadata(context.Background(), id)
					if metaErr != nil {
						// Skip if metadata not found
						continue
					}

					if !matchesTags(metadata.Tags, params.Tags, params.ExcludeTags) {
						continue
					}
				}

				matchingImages = append(matchingImages, *obj.Key)
			}

			logger.Info("Found matching images from S3 listing",
				zap.Int("count", len(matchingImages)))
		}

		if len(matchingImages) == 0 {
			logger.Warn("No images found matching criteria",
				zap.Strings("tags", params.Tags),
				zap.Strings("exclude_tags", params.ExcludeTags),
				zap.String("orientation", orientation))
			errors.HandleError(w, errors.ErrNotFound, "No images found matching criteria", nil)
			return
		}

		// Select random image
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		randomIndex := rng.Intn(len(matchingImages))
		originalKey := matchingImages[randomIndex]
		logger.Debug("Selected random image", zap.String("key", originalKey))

		// Extract filename for format path generation
		fileBaseName := filepath.Base(originalKey)
		filename := strings.TrimSuffix(fileBaseName, filepath.Ext(fileBaseName))

		// Determine best format
		bestFormat := detectBestFormat(r)
		if params.Format != "" {
			// Override with user preference if valid
			switch params.Format {
			case "avif", "webp", "original":
				bestFormat = params.Format
			}
		}

		// Handle PNG transparency preservation
		isPNG := strings.HasSuffix(strings.ToLower(originalKey), ".png")
		if isPNG && bestFormat == FormatOriginal {
			serveS3Image(s3Client, cfg, w, r, originalKey, "image/png")
			return
		}

		// Try preferred format first
		imageKey := getFormattedImagePath(bestFormat, orientation, filename)
		contentType := getContentType(bestFormat, imageKey)

		// Try to serve the preferred format
		data, err := s3Client.GetObject(r.Context(), &s3.GetObjectInput{
			Bucket: aws.String(cfg.S3Bucket),
			Key:    aws.String(imageKey),
		})

		if err != nil {
			// Fall back to original if preferred format not available
			logger.Info("Preferred format not available, falling back to original",
				zap.String("preferred", bestFormat))
			serveS3Image(s3Client, cfg, w, r, originalKey, getContentType(FormatOriginal, originalKey))
			return
		}
		defer data.Body.Close()

		// Serve the image
		setImageResponseHeaders(w, contentType)
		if _, err := io.Copy(w, data.Body); err != nil {
			logger.Error("Failed to send image", zap.Error(err))
		}
	}
}

// serveS3Image is a helper function to serve images from S3
func serveS3Image(s3Client *s3.Client, cfg *config.Config, w http.ResponseWriter, r *http.Request, key string, contentType string) {
	data, err := s3Client.GetObject(r.Context(), &s3.GetObjectInput{
		Bucket: aws.String(cfg.S3Bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		logger.Error("Failed to get image from S3", zap.String("key", key), zap.Error(err))
		errors.HandleError(w, errors.ErrNotFound, "Image not found", err)
		return
	}
	defer data.Body.Close()

	setImageResponseHeaders(w, contentType)
	if _, err := io.Copy(w, data.Body); err != nil {
		logger.Error("Failed to send image", zap.Error(err))
	}
}

// LocalRandomImageHandler serves random images from local storage
func LocalRandomImageHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters
		params := parseRandomQueryParams(r)

		// Determine device type and orientation
		deviceType := utils.DetectDeviceType(r)
		orientation := "landscape" // Default for desktop
		if deviceType == utils.DeviceMobile {
			orientation = "portrait" // Mobile gets portrait
		}

		// Override orientation if specified in params
		if params.Orientation != "" {
			orientation = params.Orientation
		}

		logger.Info("Processing random image request",
			zap.Strings("tags", params.Tags),
			zap.Strings("exclude_tags", params.ExcludeTags),
			zap.String("orientation", orientation),
			zap.String("device_type", deviceType))

		// Find matching images
		var matchingImages []*utils.ImageMetadata
		var err error

		// Use Redis for efficient filtering if available and filters are specified
		if (len(params.Tags) > 0 || len(params.ExcludeTags) > 0) && utils.IsRedisMetadataStore() {
			var candidateIDs []string

			if len(params.Tags) > 0 {
				// Get images that have ALL required tags
				candidateIDs, err = utils.GetImagesByMultipleTags(context.Background(), params.Tags)
				if err != nil {
					logger.Error("Failed to get images by tags from Redis", zap.Error(err))
				}
			} else {
				// Get all image IDs if only exclude filters are specified
				candidateIDs, err = utils.GetAllImageIDs(context.Background())
				if err != nil {
					logger.Error("Failed to get all image IDs from Redis", zap.Error(err))
				}
			}

			if err == nil && len(candidateIDs) > 0 {
				// Filter by metadata
				for _, id := range candidateIDs {
					metadata, metaErr := utils.MetadataManager.GetMetadata(context.Background(), id)
					if metaErr != nil {
						continue
					}

					// Check tag matching
					if !matchesTags(metadata.Tags, params.Tags, params.ExcludeTags) {
						continue
					}

					// Check orientation
					if metadata.Orientation == orientation {
						matchingImages = append(matchingImages, metadata)
					}
				}

				logger.Info("Found matching images from Redis",
					zap.Int("count", len(matchingImages)))
			}
		}

		// Fall back to directory scanning if Redis didn't work or no results
		if len(matchingImages) == 0 {
			// Read files from the orientation directory
			originalDir := filepath.Join(cfg.ImageBasePath, "original", orientation)
			logger.Debug("Looking for images in directory", zap.String("dir", originalDir))

			files, err := os.ReadDir(originalDir)
			if err != nil {
				logger.Error("Failed to read directory",
					zap.String("dir", originalDir),
					zap.Error(err))
				errors.HandleError(w, errors.ErrNotFound, "No images found", err)
				return
			}

			// Process each file
			for _, file := range files {
				if file.IsDir() || !utils.IsImageFile(file.Name()) {
					continue
				}

				id := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))

				// Apply tag filtering if specified
				if len(params.Tags) > 0 || len(params.ExcludeTags) > 0 {
					metadata, metaErr := utils.MetadataManager.GetMetadata(context.Background(), id)
					if metaErr != nil {
						// Skip if metadata not available
						continue
					}

					if !matchesTags(metadata.Tags, params.Tags, params.ExcludeTags) {
						continue
					}

					matchingImages = append(matchingImages, metadata)
				} else {
					// No tag filtering, create basic metadata
					matchingImages = append(matchingImages, &utils.ImageMetadata{
						ID:          id,
						Orientation: orientation,
						Paths: struct {
							Original string `json:"original"`
							WebP     string `json:"webp"`
							AVIF     string `json:"avif"`
						}{
							Original: filepath.Join("original", orientation, file.Name()),
						},
					})
				}
			}

			logger.Info("Found matching images from directory scan",
				zap.Int("count", len(matchingImages)))
		}

		if len(matchingImages) == 0 {
			logger.Warn("No images found matching criteria",
				zap.Strings("tags", params.Tags),
				zap.Strings("exclude_tags", params.ExcludeTags),
				zap.String("orientation", orientation))
			errors.HandleError(w, errors.ErrNotFound, "No images found matching criteria", nil)
			return
		}

		// Select random image
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		randomIndex := rng.Intn(len(matchingImages))
		selectedImage := matchingImages[randomIndex]
		logger.Debug("Selected random image",
			zap.String("id", selectedImage.ID),
			zap.String("orientation", selectedImage.Orientation))

		// Determine best format
		bestFormat := detectBestFormat(r)
		if params.Format != "" {
			// Override with user preference if valid
			switch params.Format {
			case "avif", "webp", "original":
				bestFormat = params.Format
			}
		}
		logger.Debug("Best format for client", zap.String("format", bestFormat))

		// Get image path and content type
		var imagePath string
		var contentType string

		// Check if the image is PNG for transparency preservation
		isPNG := false
		if selectedImage.Format == "png" {
			isPNG = true
		} else {
			isPNG = strings.HasSuffix(strings.ToLower(selectedImage.Paths.Original), ".png")
		}

		// Handle PNG transparency preservation
		if isPNG && bestFormat == FormatOriginal {
			imagePath = filepath.Join(cfg.ImageBasePath, selectedImage.Paths.Original)
			contentType = "image/png"
			logger.Debug("Using original PNG for transparency", zap.String("path", imagePath))
		} else {
			// Use the appropriate format based on browser support and preference
			switch bestFormat {
			case FormatAVIF:
				imagePath = filepath.Join(cfg.ImageBasePath, selectedImage.Orientation, "avif", selectedImage.ID+".avif")
				contentType = "image/avif"
			case FormatWebP:
				imagePath = filepath.Join(cfg.ImageBasePath, selectedImage.Orientation, "webp", selectedImage.ID+".webp")
				contentType = "image/webp"
			default:
				imagePath = filepath.Join(cfg.ImageBasePath, selectedImage.Paths.Original)
				contentType = getContentType(FormatOriginal, imagePath)
			}

			logger.Debug("Using format and path",
				zap.String("format", bestFormat),
				zap.String("path", imagePath))

			// Check if file exists, fall back to original if needed
			if _, err := os.Stat(imagePath); os.IsNotExist(err) && bestFormat != FormatOriginal {
				logger.Info("Format not available, falling back to original",
					zap.String("format", bestFormat))
				imagePath = filepath.Join(cfg.ImageBasePath, selectedImage.Paths.Original)
				contentType = getContentType(FormatOriginal, imagePath)
			}
		}

		// Read and serve the image
		imageData, err := os.ReadFile(imagePath)
		if err != nil {
			logger.Error("Failed to read image",
				zap.String("path", imagePath),
				zap.Error(err))
			errors.HandleError(w, errors.ErrNotFound, "Image not found", err)
			return
		}

		// Set response headers and send image
		setImageResponseHeaders(w, contentType)
		if _, err := w.Write(imageData); err != nil {
			logger.Error("Failed to send image", zap.Error(err))
		}
	}
}
