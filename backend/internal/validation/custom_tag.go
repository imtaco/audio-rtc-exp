package validation

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

var roomIDRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

func init() {
	MustRegisterGin("roomid", ValidateRoomID)
	MustRegisterGinAlias("userid", "uuid4")
	MustRegisterGinAlias("modules", "oneof=mixers januses")
	MustRegisterGinAlias("moduleid", "alphanum,min=3,max=32")
	MustRegisterGinAlias("role", "oneof=host guest anchor")
	MustRegisterGinAlias("label", "oneof=ready cordon draining drained unready")
}

// ValidateRoomID validates room ID format: 3-32 characters, alphanumeric with hyphens and underscores
func ValidateRoomID(fl validator.FieldLevel) bool {
	// binding.Validator =
	return roomIDRegex.MatchString(fl.Field().String())
}
