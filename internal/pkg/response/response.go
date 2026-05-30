package response

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apierrors "github.com/singll/bellkeeper/internal/pkg/errors"
)

// Success sends a 200 response with data payload.
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// Raw sends a response with a custom status code and raw data (no wrapping).
// Use for proxy passthrough endpoints that return upstream API raw responses.
func Raw(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// Created sends a 201 response with data payload.
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{"data": data})
}

// Page sends a paginated 200 response.
func Page(c *gin.Context, data interface{}, total int64, page, perPage int) {
	c.JSON(http.StatusOK, gin.H{
		"data":     data,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// Deleted sends a 200 response with delete confirmation.
func Deleted(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// Message sends a 200 response with a message string.
func Message(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

// Error sends an error response with the given HTTP status code.
func Error(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// BadRequest sends a 400 error response.
func BadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": msg})
}

// NotFound sends a 404 error response.
func NotFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, gin.H{"error": msg})
}

// InternalError sends a 500 error response.
func InternalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}

// Unauthorized sends a 401 error response.
func Unauthorized(c *gin.Context, msg string) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": msg})
}

// Forbidden sends a 403 error response.
func Forbidden(c *gin.Context, msg string) {
	c.JSON(http.StatusForbidden, gin.H{"error": msg})
}

// TooManyRequests sends a 429 error response.
func TooManyRequests(c *gin.Context, msg string) {
	c.JSON(http.StatusTooManyRequests, gin.H{"error": msg})
}

// ParsePagination extracts page and perPage from query parameters with defaults.
func ParsePagination(c *gin.Context) (page, perPage int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ = strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	return
}

// ParseID extracts and validates an unsigned integer ID from a URL parameter.
// Returns the parsed ID and true on success, or 0 and false on failure (error response already sent).
func ParseID(c *gin.Context, param string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(param), 10, 32)
	if err != nil {
		BadRequest(c, "invalid "+param)
		return 0, false
	}
	return uint(id), true
}

// ErrorFromService automatically maps a ServiceError to the appropriate HTTP response.
// Falls back to InternalError if the error is not a ServiceError.
func ErrorFromService(c *gin.Context, err error) {
	if err == nil {
		return
	}

	// Try to extract ServiceError from the error chain
	if svcErr := apierrors.GetServiceError(err); svcErr != nil {
		Error(c, svcErr.Code, svcErr.Message)
		return
	}

	// Fallback: check for sentinel errors
	if apierrors.IsNotFound(err) {
		NotFound(c, "resource not found")
		return
	}
	if apierrors.IsInvalidInput(err) {
		BadRequest(c, "invalid input")
		return
	}
	if apierrors.Is(err, apierrors.ErrUnauthorized) {
		Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	if apierrors.Is(err, apierrors.ErrRateLimited) {
		Error(c, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	// Default fallback
	InternalError(c, err.Error())
}
