package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for common error conditions
var (
	// NotFound indicates a requested resource was not found
	ErrNotFound = errors.New("not found")

	// AlreadyExists indicates a resource already exists (e.g., duplicate key)
	ErrAlreadyExists = errors.New("already exists")

	// InvalidInput indicates invalid input parameters
	ErrInvalidInput = errors.New("invalid input")

	// InternalError indicates an internal error occurred
	ErrInternal = errors.New("internal error")

	// Unauthorized indicates the request is not authorized
	ErrUnauthorized = errors.New("unauthorized")

	// RateLimited indicates rate limit exceeded
	ErrRateLimited = errors.New("rate limited")
)

// ServiceError wraps an error with operation context and HTTP status code mapping.
type ServiceError struct {
	Op      string // Operation name, e.g. "ragflow.upload"
	Code    int    // HTTP status code mapping
	Err     error  // Original error
	Message string // User-facing message
}

func (e *ServiceError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Message)
}

func (e *ServiceError) Unwrap() error {
	return e.Err
}

// NewServiceError creates a new ServiceError with the given operation and error.
// HTTP status code is inferred from the error type.
func NewServiceError(op string, err error) *ServiceError {
	code := http.StatusInternalServerError
	msg := "internal error"

	if err == nil {
		return &ServiceError{Op: op, Code: code, Message: msg}
	}

	// Infer HTTP status code from error type
	var codeErr interface{ HTTPStatusCode() int }
	if errors.As(err, &codeErr) {
		code = codeErr.HTTPStatusCode()
	} else if errors.Is(err, ErrNotFound) {
		code = http.StatusNotFound
		msg = "not found"
	} else if errors.Is(err, ErrAlreadyExists) {
		code = http.StatusConflict
		msg = "resource already exists"
	} else if errors.Is(err, ErrInvalidInput) {
		code = http.StatusBadRequest
		msg = "invalid input"
	} else if errors.Is(err, ErrUnauthorized) {
		code = http.StatusUnauthorized
		msg = "unauthorized"
	} else if errors.Is(err, ErrRateLimited) {
		code = http.StatusTooManyRequests
		msg = "rate limit exceeded"
	} else {
		msg = err.Error()
	}

	return &ServiceError{
		Op:      op,
		Code:    code,
		Err:     err,
		Message: msg,
	}
}

// WithMessage creates a new ServiceError with a custom user-facing message.
func (e *ServiceError) WithMessage(msg string) *ServiceError {
	e.Message = msg
	return e
}

// HTTPStatusCode allows ServiceError to implement an interface for HTTP status code extraction.
func (e *ServiceError) HTTPStatusCode() int {
	return e.Code
}

// WithContext wraps an error with additional context information.
func WithContext(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// WithField adds a field to the error message without wrapping.
func WithField(err error, field, value string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w (field %s=%s)", err, field, value)
}

// IsNotFound checks if the error is a not-found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExists checks if the error is an already-exists error.
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// IsInvalidInput checks if the error is an invalid-input error.
func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}

// GetServiceError extracts a ServiceError from the error chain.
func GetServiceError(err error) *ServiceError {
	var svcErr *ServiceError
	if errors.As(err, &svcErr) {
		return svcErr
	}
	return nil
}

// Is checks if the target error matches this error (for errors.Is compatibility).
func Is(err, target error) bool {
	if target == nil {
		return err == nil
	}
	return errors.Is(err, target)
}
