package grpcApi

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"lazervaultGo/pb"
	"lazervaultGo/services"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FacialRecognitionController struct {
	service *services.FacialRecognitionService
}

func NewFacialRecognitionController(service *services.FacialRecognitionService) *FacialRecognitionController {
	return &FacialRecognitionController{
		service: service,
	}
}

// RegisterFace handles face registration via REST API
func (c *FacialRecognitionController) RegisterFace(ctx *gin.Context) {
	// Parse multipart form
	err := ctx.Request.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid multipart form data",
		})
		return
	}

	// Extract form values
	userID := ctx.PostForm("user_id")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "user_id is required",
		})
		return
	}

	faceID := ctx.PostForm("face_id")
	allowDuplicatesStr := ctx.PostForm("allow_duplicates")
	duplicateThresholdStr := ctx.PostForm("duplicate_threshold")

	// Parse boolean and float values
	allowDuplicates, _ := strconv.ParseBool(allowDuplicatesStr)
	duplicateThreshold, _ := strconv.ParseFloat(duplicateThresholdStr, 64)
	if duplicateThreshold == 0 {
		duplicateThreshold = 0.85 // Default threshold
	}

	// Extract image file
	file, header, err := ctx.Request.FormFile("image")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "image file is required",
		})
		return
	}
	defer file.Close()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "file must be an image",
		})
		return
	}

	// Read image data
	imageData, err := io.ReadAll(file)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to read image data",
		})
		return
	}

	// Create gRPC request
	req := &pb.RegisterFaceRequest{
		UserId:             userID,
		FaceId:             faceID,
		AllowDuplicates:    allowDuplicates,
		DuplicateThreshold: duplicateThreshold,
		ImageData:          imageData,
		ImageFilename:      header.Filename,
		ImageContentType:   contentType,
	}

	// Call service
	resp, err := c.service.RegisterFace(ctx.Request.Context(), req)
	if err != nil {
		c.handleError(ctx, err)
		return
	}

	// Convert response to JSON
	jsonResp := gin.H{
		"success":            resp.Success,
		"face_id":            resp.FaceId,
		"message":            resp.Message,
		"error":              resp.Error,
		"num_faces_detected": resp.NumFacesDetected,
	}

	if resp.DuplicateDetails != nil {
		jsonResp["duplicate_details"] = gin.H{
			"is_duplicate":  resp.DuplicateDetails.IsDuplicate,
			"threshold":     resp.DuplicateDetails.Threshold,
			"total_matches": resp.DuplicateDetails.TotalMatches,
			"message":       resp.DuplicateDetails.Message,
			"security_note": resp.DuplicateDetails.SecurityNote,
		}

		if resp.DuplicateDetails.PrimaryMatch != nil {
			jsonResp["duplicate_details"].(gin.H)["primary_match"] = gin.H{
				"user_id":       resp.DuplicateDetails.PrimaryMatch.UserId,
				"face_id":       resp.DuplicateDetails.PrimaryMatch.FaceId,
				"confidence":    resp.DuplicateDetails.PrimaryMatch.Confidence,
				"registered_at": resp.DuplicateDetails.PrimaryMatch.RegisteredAt,
			}
		}

		if len(resp.DuplicateDetails.AllMatches) > 0 {
			allMatches := make([]gin.H, len(resp.DuplicateDetails.AllMatches))
			for i, match := range resp.DuplicateDetails.AllMatches {
				allMatches[i] = gin.H{
					"user_id":       match.UserId,
					"face_id":       match.FaceId,
					"confidence":    match.Confidence,
					"registered_at": match.RegisteredAt,
				}
			}
			jsonResp["duplicate_details"].(gin.H)["all_matches"] = allMatches
		}
	}

	ctx.JSON(http.StatusOK, jsonResp)
}

// VerifyFace handles face verification via REST API
func (c *FacialRecognitionController) VerifyFace(ctx *gin.Context) {
	// Parse multipart form
	err := ctx.Request.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid multipart form data",
		})
		return
	}

	// Extract form values
	userID := ctx.PostForm("user_id")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "user_id is required",
		})
		return
	}

	thresholdStr := ctx.PostForm("threshold")
	threshold, _ := strconv.ParseFloat(thresholdStr, 64)
	if threshold == 0 {
		threshold = 0.7 // Default threshold
	}

	// Extract image file
	file, header, err := ctx.Request.FormFile("image")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "image file is required",
		})
		return
	}
	defer file.Close()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "file must be an image",
		})
		return
	}

	// Read image data
	imageData, err := io.ReadAll(file)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to read image data",
		})
		return
	}

	// Create gRPC request
	req := &pb.VerifyFaceRequest{
		UserId:           userID,
		Threshold:        threshold,
		ImageData:        imageData,
		ImageFilename:    header.Filename,
		ImageContentType: contentType,
	}

	// Call service
	resp, err := c.service.VerifyFace(ctx.Request.Context(), req)
	if err != nil {
		c.handleError(ctx, err)
		return
	}

	// Convert response to JSON
	jsonResp := gin.H{
		"success":         resp.Success,
		"verified":        resp.Verified,
		"confidence":      resp.Confidence,
		"matched_face_id": resp.MatchedFaceId,
		"threshold":       resp.Threshold,
		"distance":        resp.Distance,
		"message":         resp.Message,
		"error":           resp.Error,
	}

	ctx.JSON(http.StatusOK, jsonResp)
}

// HealthCheck handles health check requests
func (c *FacialRecognitionController) HealthCheck(ctx *gin.Context) {
	req := &pb.HealthCheckRequest{}
	resp, err := c.service.HealthCheck(ctx.Request.Context(), req)
	if err != nil {
		c.handleError(ctx, err)
		return
	}

	statusCode := http.StatusOK
	if !resp.Healthy {
		statusCode = http.StatusServiceUnavailable
	}

	ctx.JSON(statusCode, gin.H{
		"healthy":         resp.Healthy,
		"message":         resp.Message,
		"service_version": resp.ServiceVersion,
		"timestamp":       resp.Timestamp,
	})
}

// handleError converts gRPC errors to HTTP errors
func (c *FacialRecognitionController) handleError(ctx *gin.Context, err error) {
	st, ok := status.FromError(err)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Internal server error",
		})
		return
	}

	var statusCode int
	switch st.Code() {
	case codes.InvalidArgument:
		statusCode = http.StatusBadRequest
	case codes.Unauthenticated:
		statusCode = http.StatusUnauthorized
	case codes.PermissionDenied:
		statusCode = http.StatusForbidden
	case codes.NotFound:
		statusCode = http.StatusNotFound
	case codes.AlreadyExists:
		statusCode = http.StatusConflict
	case codes.ResourceExhausted:
		statusCode = http.StatusTooManyRequests
	case codes.FailedPrecondition:
		statusCode = http.StatusUnprocessableEntity
	case codes.Unavailable:
		statusCode = http.StatusServiceUnavailable
	default:
		statusCode = http.StatusInternalServerError
	}

	ctx.JSON(statusCode, gin.H{
		"success": false,
		"error":   st.Message(),
	})
}

// Middleware for rate limiting
func (c *FacialRecognitionController) RateLimitMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		// Simple rate limiting - you can implement more sophisticated logic here
		// For now, just log the request
		log.Printf("Facial recognition request from %s", ctx.ClientIP())
		ctx.Next()
	})
}

// Middleware for authentication
func (c *FacialRecognitionController) AuthMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		// Check for API key or JWT token
		apiKey := ctx.GetHeader("X-API-Key")
		authHeader := ctx.GetHeader("Authorization")

		if apiKey == "" && authHeader == "" {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authentication required",
			})
			ctx.Abort()
			return
		}

		// Add authentication logic here
		// For now, just continue
		ctx.Next()
	})
}

// CORS middleware
func (c *FacialRecognitionController) CORSMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Credentials", "true")
		ctx.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-API-Key")
		ctx.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")

		if ctx.Request.Method == "OPTIONS" {
			ctx.AbortWithStatus(204)
			return
		}

		ctx.Next()
	})
}

// Metrics middleware
func (c *FacialRecognitionController) MetricsMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		// Add metrics collection here
		// For now, just log the request
		log.Printf("Facial recognition API request: %s %s", ctx.Request.Method, ctx.Request.URL.Path)
		ctx.Next()
	})
}

// Security headers middleware
func (c *FacialRecognitionController) SecurityHeadersMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		ctx.Header("X-Content-Type-Options", "nosniff")
		ctx.Header("X-Frame-Options", "DENY")
		ctx.Header("X-XSS-Protection", "1; mode=block")
		ctx.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		ctx.Header("Content-Security-Policy", "default-src 'self'")
		ctx.Next()
	})
}

// Custom validation middleware for image files
func (c *FacialRecognitionController) ImageValidationMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		// Check if request has multipart form
		if strings.Contains(ctx.GetHeader("Content-Type"), "multipart/form-data") {
			// Parse form to validate image
			err := ctx.Request.ParseMultipartForm(32 << 20)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "Invalid multipart form data",
				})
				ctx.Abort()
				return
			}

			// Check if image file exists
			_, _, err = ctx.Request.FormFile("image")
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "Image file is required",
				})
				ctx.Abort()
				return
			}
		}

		ctx.Next()
	})
}
