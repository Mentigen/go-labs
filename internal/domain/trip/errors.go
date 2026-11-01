package trip

import (
	"errors"
	"fmt"
)

var (
	ErrTripNotFound        = errors.New("trip not found")
	ErrTripCompleted       = errors.New("trip already completed")
	ErrDriverBusy          = errors.New("driver already has an active trip")
	ErrIdempotencyConflict = errors.New("idempotency key already used with a different request body")
)

type ValidationError struct {
	Field  string
	Reason string
}

func NewValidationError(field, reason string) *ValidationError {
	return &ValidationError{Field: field, Reason: reason}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Reason)
}
