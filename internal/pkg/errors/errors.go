package errors

import (
	"errors"
	"fmt"
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
