package response

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Success sends a 200 response with data payload.
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"data": data})
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
