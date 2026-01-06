package validation

import "github.com/go-playground/validator/v10"

func FormatValidationError(err error) []Error {
	var errors []Error
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			errors = append(errors, Error{
				Field:   e.Field(),
				Message: e.Error(), // Use built-in error message
			})
		}
	}

	return errors
}

// Error represents a validation error
type Error struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
