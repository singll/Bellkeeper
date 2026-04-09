package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithContext(t *testing.T) {
	baseErr := errors.New("original error")

	tests := []struct {
		name    string
		err     error
		context string
		want    string
	}{
		{
			name:    "wrap error",
			err:     baseErr,
			context: "operation failed",
			want:    "operation failed: original error",
		},
		{
			name:    "nil error",
			err:     nil,
			context: "operation",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WithContext(tt.err, tt.context)
			if tt.err == nil {
				assert.Nil(t, got)
			} else {
				assert.Contains(t, got.Error(), tt.want)
			}
		})
	}
}

func TestWithField(t *testing.T) {
	baseErr := errors.New("validation error")

	got := WithField(baseErr, "field", "name")
	assert.Contains(t, got.Error(), "validation error")
	assert.Contains(t, got.Error(), "field")
	assert.Contains(t, got.Error(), "name")
}

func TestIsNotFound(t *testing.T) {
	assert.True(t, IsNotFound(ErrNotFound))
	assert.False(t, IsNotFound(ErrInvalidInput))
	assert.False(t, IsNotFound(nil))

	// Wrap ErrNotFound should also match
	wrapped := fmt.Errorf("database: %w", ErrNotFound)
	assert.True(t, IsNotFound(wrapped))
}

func TestIsAlreadyExists(t *testing.T) {
	assert.True(t, IsAlreadyExists(ErrAlreadyExists))
	assert.False(t, IsAlreadyExists(ErrNotFound))
}

func TestIsInvalidInput(t *testing.T) {
	assert.True(t, IsInvalidInput(ErrInvalidInput))
	assert.False(t, IsInvalidInput(ErrNotFound))
}
