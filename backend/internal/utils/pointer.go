package utils

// Ptr returns a pointer to the passed value.
func Ptr[T any](t T) *T {
	return &t
}

func Get[T any](t *T) T {
	if t == nil {
		var v T
		return v
	}
	return *t
}
