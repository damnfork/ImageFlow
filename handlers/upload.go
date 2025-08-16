package handlers

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yuri-NagaSaki/ImageFlow/config"
	"github.com/Yuri-NagaSaki/ImageFlow/utils"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/errors"
	"github.com/Yuri-NagaSaki/ImageFlow/utils/logger"
	"go.uber.org/zap"
)

// UploadResult represents the result of an image upload
type UploadResult struct {
	Filename    string            `json:"filename"`
	Status      string            `json:"status"`
	Message     string            `json:"message"`
	Orientation string            `json:"orientation,omitempty"`
	Format      string            `json:"format,omitempty"`
	URLs        map[string]string `json:"urls,omitempty"`
	ExpiryTime  string            `json:"expiryTime,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

// getPublicURL constructs a public-facing URL for accessing an image
func getPublicURL(key string, cfg *config.Config) string {
	if cfg.StorageType == config.StorageTypeLocal {
		return fmt.Sprintf("/images/%s", key)
	}
	// For S3 storage
	if cfg.CustomDomain != "" {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(cfg.CustomDomain, "/"), key)
	}
	// Fallback to S3 endpoint with bucket name
	endpoint := strings.TrimSuffix(cfg.S3Endpoint, "/")
	return fmt.Sprintf("%s/%s/%s", endpoint, cfg.S3Bucket, key)
}

// determineImageOrientation classifies an image as landscape or portrait
// Square images and portrait images are classified as portrait
func determineImageOrientation(img image.Config) string {
	if img.Width > img.Height {
		return "landscape"
	}
	return "portrait"
}

// processImage handles the processing of a single image file
func processImage(ctx *uploadContext, fileHeader *multipart.FileHeader) UploadResult {
	file, err := fileHeader.Open()
	if err != nil {
		logger.Error("打开上传文件失败",
			zap.String("filename", fileHeader.Filename),
			zap.Error(err))
		return UploadResult{
			Filename: fileHeader.Filename,
			Status:   "error",
			Message:  "打开文件失败",
		}
	}
	defer file.Close()

	// Read image configuration to determine orientation
	img, _, err := image.DecodeConfig(file)
	if err != nil {
		return UploadResult{
			Filename: fileHeader.Filename,
			Status:   "error",
			Message:  fmt.Sprintf("Error reading image configuration: %v", err),
		}
	}
	orientation := determineImageOrientation(img)

	// Reset file pointer
	if _, err := file.Seek(0, 0); err != nil {
		return UploadResult{
			Filename: fileHeader.Filename,
			Status:   "error",
			Message:  fmt.Sprintf("Error resetting file pointer: %v", err),
		}
	}

	// Read file content
	data := make([]byte, fileHeader.Size)
	if _, err := file.Read(data); err != nil {
		return UploadResult{
			Filename: fileHeader.Filename,
			Status:   "error",
			Message:  fmt.Sprintf("Error reading file: %v", err),
		}
	}

	// Generate unique filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%d", timestamp, time.Now().UnixNano()%10000)
	imageID := filename

	// Detect image format
	imgFormat, err := utils.DetectImageFormat(data)
	if err != nil {
		return UploadResult{
			Filename: fileHeader.Filename,
			Status:   "error",
			Message:  fmt.Sprintf("Error detecting image format: %v", err),
		}
	}

	// Create user storage paths manager
	userPaths := utils.NewUserStoragePaths(ctx.user.ID, ctx.cfg)

	// Ensure user directories exist
	if err := userPaths.EnsureUserDirectories(); err != nil {
		return UploadResult{
			Filename: fileHeader.Filename,
			Status:   "error",
			Message:  fmt.Sprintf("Error creating user directories: %v", err),
		}
	}

	// Generate storage paths using user-specific paths
	var originalKey string
	if imgFormat.Format == "gif" {
		originalKey = userPaths.GetGIFPath(filename + imgFormat.Extension)
	} else {
		originalKey = userPaths.GetOriginalPath(filename+imgFormat.Extension, orientation)
	}

	if err := utils.Storage.Store(ctx.r.Context(), originalKey, data); err != nil {
		return UploadResult{
			Filename: fileHeader.Filename,
			Status:   "error",
			Message:  fmt.Sprintf("Error storing original file: %v", err),
		}
	}
	logger.Info("Original image stored",
		zap.String("key", originalKey),
		zap.String("filename", fileHeader.Filename),
		zap.String("format", imgFormat.Format),
		zap.Int("size", len(data)))

	var originalSize, webpSize, avifSize int64
	originalSize = int64(len(data))

	var webpURL, avifURL string
	var wg sync.WaitGroup

	if imgFormat.Format != "gif" {
		// WebP conversion
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Debug("Starting WebP conversion",
				zap.String("filename", fileHeader.Filename))

			webpData, err := utils.ConvertToWebPWithBimg(data, ctx.cfg)
			if err != nil {
				logger.Error("WebP conversion failed",
					zap.String("filename", fileHeader.Filename),
					zap.Error(err))
				return
			}

			webpKey := userPaths.GetWebPPath(filename, orientation)
			if err := utils.Storage.Store(ctx.r.Context(), webpKey, webpData); err != nil {
				logger.Error("Failed to store WebP image",
					zap.String("key", webpKey),
					zap.Error(err))
				return
			}

			webpURL = getPublicURL(webpKey, ctx.cfg)
			webpSize = int64(len(webpData))
			logger.Info("WebP conversion completed",
				zap.String("key", webpKey),
				zap.String("url", webpURL),
				zap.Int64("size", webpSize))
		}()

		// AVIF conversion
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Debug("Starting AVIF conversion",
				zap.String("filename", fileHeader.Filename))

			avifData, err := utils.ConvertToAVIFWithBimg(data, ctx.cfg)
			if err != nil {
				logger.Error("AVIF conversion failed",
					zap.String("filename", fileHeader.Filename),
					zap.Error(err))
				return
			}

			avifKey := userPaths.GetAVIFPath(filename, orientation)
			if err := utils.Storage.Store(ctx.r.Context(), avifKey, avifData); err != nil {
				logger.Error("Failed to store AVIF image",
					zap.String("key", avifKey),
					zap.Error(err))
				return
			}

			avifURL = getPublicURL(avifKey, ctx.cfg)
			avifSize = int64(len(avifData))
			logger.Info("AVIF conversion completed",
				zap.String("key", avifKey),
				zap.String("url", avifURL),
				zap.Int64("size", avifSize))
		}()

		wg.Wait()
	} else {
		logger.Info("Skipping conversions for GIF image",
			zap.String("filename", fileHeader.Filename))
		// For GIF, all formats use the same file
		webpSize = originalSize
		avifSize = originalSize
	}

	// Get URL for original image
	originalURL := getPublicURL(originalKey, ctx.cfg)

	// Set WebP and AVIF URLs with defaults if conversion failed
	if webpURL == "" {
		logger.Debug("Using original URL for WebP",
			zap.String("filename", fileHeader.Filename))
		webpURL = originalURL
	}
	if avifURL == "" {
		logger.Debug("Using original URL for AVIF",
			zap.String("filename", fileHeader.Filename))
		avifURL = originalURL
	}

	var expiryTimeStr string
	if !ctx.expiryTime.IsZero() {
		expiryTimeStr = ctx.expiryTime.Format(time.RFC3339)
	}

	metadata := &utils.ImageMetadata{
		ID:           imageID,
		UserID:       ctx.user.ID,
		OriginalName: fileHeader.Filename,
		UploadTime:   time.Now(),
		Format:       imgFormat.Format,
		Orientation:  orientation,
		Tags:         ctx.tags,
		Sizes:        make(map[string]int64),
	}

	if !ctx.expiryTime.IsZero() {
		metadata.ExpiryTime = ctx.expiryTime
	}

	// Set paths using user-specific paths
	metadata.Paths.Original = originalKey
	if webpURL != originalURL {
		metadata.Paths.WebP = userPaths.GetWebPPath(imageID, orientation)
	}
	if avifURL != originalURL {
		metadata.Paths.AVIF = userPaths.GetAVIFPath(imageID, orientation)
	}

	// Set file sizes - always store the actual sizes
	metadata.Sizes["original"] = originalSize
	if webpSize > 0 {
		metadata.Sizes["webp"] = webpSize
	} else {
		// If WebP conversion failed, use original size as fallback
		metadata.Sizes["webp"] = originalSize
	}
	if avifSize > 0 {
		metadata.Sizes["avif"] = avifSize
	} else {
		// If AVIF conversion failed, use original size as fallback
		metadata.Sizes["avif"] = originalSize
	}

	if err := utils.MetadataManager.SaveMetadata(ctx.r.Context(), metadata); err != nil {
		logger.Warn("Failed to save metadata",
			zap.String("image_id", imageID),
			zap.Error(err))
	} else {
		logger.Debug("Metadata saved successfully",
			zap.String("image_id", imageID),
			zap.String("format", imgFormat.Format),
			zap.String("orientation", orientation))
	}

	return UploadResult{
		Filename:    fileHeader.Filename,
		Status:      "success",
		Message:     "File uploaded and converted successfully",
		Orientation: orientation,
		Format:      imgFormat.Format,
		ExpiryTime:  expiryTimeStr,
		Tags:        ctx.tags,
		URLs: map[string]string{
			"original": originalURL,
			"webp":     webpURL,
			"avif":     avifURL,
		},
	}
}

type uploadContext struct {
	r          *http.Request
	user       *utils.User
	expiryTime time.Time
	tags       []string
	cfg        *config.Config
}

// UploadHandler handles image uploads, converting them to multiple formats
func UploadHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			errors.HandleError(w, errors.ErrInvalidParam, "方法不允许", nil)
			return
		}

		// Get user from context (set by RequireAuth middleware)
		user, ok := GetUserFromContext(r.Context())
		if !ok {
			errors.HandleError(w, errors.ErrUnauthorized, "用户未认证", nil)
			return
		}

		// Parse multipart form with default max upload size (32MB)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			logger.Error("解析表单失败", zap.Error(err))
			errors.HandleError(w, errors.ErrInvalidParam, "解析表单失败", nil)
			return
		}

		// Get uploaded files
		files := r.MultipartForm.File["images[]"]
		if len(files) == 0 {
			errors.HandleError(w, errors.ErrInvalidParam, "未上传文件", nil)
			return
		}

		// Check if number of uploaded files exceeds the maximum limit
		if len(files) > cfg.MaxUploadCount {
			errors.HandleError(w, errors.ErrInvalidParam,
				fmt.Sprintf("上传文件数量超过限制，最多允许上传 %d 个文件", cfg.MaxUploadCount),
				nil)
			return
		}

		// Get expiry time parameter (in minutes)
		expiryMinutes := 0 // Default: never expire
		if expiryParam := r.FormValue("expiryMinutes"); expiryParam != "" {
			if minutes, err := strconv.Atoi(expiryParam); err == nil && minutes >= 0 {
				expiryMinutes = minutes
			} else {
				logger.Warn("无效的过期时间参数",
					zap.String("expiry_minutes", expiryParam),
					zap.Int("default_value", expiryMinutes))
			}
		}

		// Calculate expiry time
		var expiryTime time.Time
		if expiryMinutes > 0 {
			expiryTime = time.Now().Add(time.Duration(expiryMinutes) * time.Minute)
			logger.Debug("设置图片过期时间",
				zap.Time("expiry_time", expiryTime),
				zap.Int("expiry_minutes", expiryMinutes))
		}

		// Get tags parameter
		var tags []string
		if tagsParam := r.FormValue("tags"); tagsParam != "" {
			// Split by comma and trim spaces
			for _, tag := range strings.Split(tagsParam, ",") {
				trimmedTag := strings.TrimSpace(tag)
				if trimmedTag != "" {
					tags = append(tags, trimmedTag)
				}
			}
			logger.Debug("图片标签", zap.Strings("tags", tags))
		}

		ctx := &uploadContext{
			r:          r,
			user:       user,
			expiryTime: expiryTime,
			tags:       tags,
			cfg:        cfg,
		}

		// Process images concurrently
		resultsChan := make(chan UploadResult, len(files))
		var wg sync.WaitGroup

		for _, fileHeader := range files {
			wg.Add(1)
			go func(fh *multipart.FileHeader) {
				defer wg.Done()
				result := processImage(ctx, fh)
				resultsChan <- result
			}(fileHeader)
		}

		// Start a goroutine to close results channel after all processing is done
		go func() {
			wg.Wait()
			close(resultsChan)
		}()

		// Collect results
		results := make([]UploadResult, 0, len(files))
		for result := range resultsChan {
			results = append(results, result)
		}

		// Return JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"results": results,
		}); err != nil {
			logger.Error("编码响应失败", zap.Error(err))
			errors.HandleError(w, errors.ErrInternal, "服务器内部错误", nil)
			return
		}
	}
}
