package validation

import (
	"errors"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func MustRegisterGin(tag string, fn validator.Func) {
	if err := RegisterGin(tag, fn); err != nil {
		panic(err)
	}
}

func MustRegisterGinAlias(tag string, alias string) {
	if err := RegisterGinAlias(tag, alias); err != nil {
		panic(err)
	}
}

func Register(v *validator.Validate, tag string, fn validator.Func) error {
	return v.RegisterValidation(tag, fn)
}

func RegisterAlias(v *validator.Validate, tag string, alias string) {
	v.RegisterAlias(tag, alias)
}

func RegisterGin(tag string, fn validator.Func) error {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		return Register(v, tag, fn)
	}
	return errors.New("validator engine is not of type *validator.Validate")
}

func RegisterGinAlias(tag string, alias string) error {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		RegisterAlias(v, tag, alias)
		return nil
	}
	return errors.New("validator engine is not of type *validator.Validate")
}
