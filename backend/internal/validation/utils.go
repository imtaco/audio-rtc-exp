package validation

import (
	"github.com/go-playground/validator/v10"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
)

func FormatValidationError(err error) []Error {
	var errs []Error

	if validationErrors, ok := errors.As[validator.ValidationErrors](err); ok {
		for _, e := range validationErrors {
			errs = append(errs, Error{
				Field:   e.Field(),
				Message: e.Error(), // Use built-in error message
			})
		}
	}

	return errs
}

// Error represents a validation error
type Error struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
