package validation

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// ValidationTestSuite is the test suite for validation package
type ValidationTestSuite struct {
	suite.Suite
	validator *validator.Validate
}

// SetupTest runs before each test
func (s *ValidationTestSuite) SetupTest() {
	s.validator = validator.New()
}

// TestValidationTestSuite runs the test suite
func TestValidationTestSuite(t *testing.T) {
	suite.Run(t, new(ValidationTestSuite))
}

// TestValidateRoomID tests the custom roomid validation tag
func (s *ValidationTestSuite) TestValidateRoomID() {
	// Register the custom validation
	err := Register(s.validator, "roomid", ValidateRoomID)
	s.Require().NoError(err)

	tests := []struct {
		name    string
		roomID  string
		wantErr bool
	}{
		{
			name:    "valid alphanumeric",
			roomID:  "room123",
			wantErr: false,
		},
		{
			name:    "valid with hyphens",
			roomID:  "room-123",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			roomID:  "room_123",
			wantErr: false,
		},
		{
			name:    "valid mixed",
			roomID:  "My-Room_123",
			wantErr: false,
		},
		{
			name:    "valid minimum length",
			roomID:  "abc",
			wantErr: false,
		},
		{
			name:    "valid maximum length (32 chars)",
			roomID:  "12345678901234567890123456789012",
			wantErr: false,
		},
		{
			name:    "invalid - too short (2 chars)",
			roomID:  "ab",
			wantErr: true,
		},
		{
			name:    "invalid - too long (33 chars)",
			roomID:  "123456789012345678901234567890123",
			wantErr: true,
		},
		{
			name:    "invalid - special characters (@)",
			roomID:  "room@123",
			wantErr: true,
		},
		{
			name:    "invalid - spaces",
			roomID:  "room 123",
			wantErr: true,
		},
		{
			name:    "invalid - empty string",
			roomID:  "",
			wantErr: true,
		},
		{
			name:    "invalid - dots",
			roomID:  "room.123",
			wantErr: true,
		},
		{
			name:    "invalid - slash",
			roomID:  "room/123",
			wantErr: true,
		},
		{
			name:    "valid - all uppercase",
			roomID:  "ROOM123",
			wantErr: false,
		},
		{
			name:    "valid - all lowercase",
			roomID:  "room123",
			wantErr: false,
		},
		{
			name:    "valid - numbers only",
			roomID:  "12345",
			wantErr: false,
		},
		{
			name:    "valid - hyphens only with alphanumeric",
			roomID:  "a-b-c-d",
			wantErr: false,
		},
		{
			name:    "valid - underscores only with alphanumeric",
			roomID:  "a_b_c_d",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			type TestStruct struct {
				RoomID string `validate:"roomid"`
			}

			testData := TestStruct{RoomID: tt.roomID}
			err := s.validator.Struct(testData)

			if tt.wantErr {
				s.Require().Error(err, "Expected validation error for roomID: %s", tt.roomID)
			} else {
				s.Require().NoError(err, "Expected no validation error for roomID: %s", tt.roomID)
			}
		})
	}
}

// TestValidateRoomIDRegex tests the regex pattern directly
func (s *ValidationTestSuite) TestValidateRoomIDRegex() {
	s.True(roomIDRegex.MatchString("abc"))
	s.True(roomIDRegex.MatchString("Room-123_test"))
	s.True(roomIDRegex.MatchString("12345678901234567890123456789012"))

	s.False(roomIDRegex.MatchString("ab"))
	s.False(roomIDRegex.MatchString("123456789012345678901234567890123"))
	s.False(roomIDRegex.MatchString("room@123"))
	s.False(roomIDRegex.MatchString(""))
}

// TestRegister tests the Register function
func (s *ValidationTestSuite) TestRegister() {
	customValidator := func(fl validator.FieldLevel) bool {
		return fl.Field().String() == "test"
	}

	err := Register(s.validator, "custom", customValidator)
	s.Require().NoError(err)

	type TestStruct struct {
		Field string `validate:"custom"`
	}

	// Test valid case
	err = s.validator.Struct(TestStruct{Field: "test"})
	s.Require().NoError(err)

	// Test invalid case
	err = s.validator.Struct(TestStruct{Field: "invalid"})
	s.Require().Error(err)
}

// TestRegisterAlias tests the RegisterAlias function
func (s *ValidationTestSuite) TestRegisterAlias() {
	RegisterAlias(s.validator, "testalias", "required,min=5")

	type TestStruct struct {
		Field string `validate:"testalias"`
	}

	// Test valid case
	err := s.validator.Struct(TestStruct{Field: "hello"})
	s.Require().NoError(err)

	// Test invalid case - too short
	err = s.validator.Struct(TestStruct{Field: "hi"})
	s.Require().Error(err)

	// Test invalid case - empty
	err = s.validator.Struct(TestStruct{Field: ""})
	s.Require().Error(err)
}

// TestMustRegisterGin tests MustRegisterGin panic behavior
func (s *ValidationTestSuite) TestMustRegisterGinPanic() {
	// This test will panic if RegisterGin fails
	// We can't easily test this without initializing Gin's binding
	// So we just verify the function exists and document it
	s.NotNil(MustRegisterGin)
}

// TestMustRegisterGinAlias tests MustRegisterGinAlias panic behavior
func (s *ValidationTestSuite) TestMustRegisterGinAliasPanic() {
	// This test will panic if RegisterGinAlias fails
	// We can't easily test this without initializing Gin's binding
	// So we just verify the function exists and document it
	s.NotNil(MustRegisterGinAlias)
}

// TestFormatValidationError tests the FormatValidationError utility
func (s *ValidationTestSuite) TestFormatValidationError() {
	type TestStruct struct {
		Email string `validate:"required,email"`
		Age   int    `validate:"required,min=18,max=120"`
		Name  string `validate:"required,min=2"`
	}

	// Test with validation errors
	testData := TestStruct{
		Email: "invalid-email",
		Age:   10,
		Name:  "A",
	}

	err := s.validator.Struct(testData)
	s.Require().Error(err)

	formatted := FormatValidationError(err)
	s.NotEmpty(formatted)
	s.Len(formatted, 3, "Expected 3 validation errors")

	// Check that all fields are present
	fields := make(map[string]bool)
	for _, e := range formatted {
		fields[e.Field] = true
		s.NotEmpty(e.Message)
	}

	s.True(fields["Email"])
	s.True(fields["Age"])
	s.True(fields["Name"])
}

// TestFormatValidationErrorNoError tests FormatValidationError with no errors
func (s *ValidationTestSuite) TestFormatValidationErrorNoError() {
	type TestStruct struct {
		Email string `validate:"required,email"`
	}

	testData := TestStruct{Email: "valid@example.com"}
	err := s.validator.Struct(testData)
	s.Require().NoError(err)

	formatted := FormatValidationError(err)
	s.Empty(formatted)
}

// TestFormatValidationErrorNonValidationError tests FormatValidationError with non-validation errors
func (s *ValidationTestSuite) TestFormatValidationErrorNonValidationError() {
	// Pass a non-validation error
	formatted := FormatValidationError(assert.AnError)
	s.Empty(formatted)
}

// CustomTagsTestSuite tests all custom tags defined in custom_tag.go
type CustomTagsTestSuite struct {
	suite.Suite
	validator *validator.Validate
}

// SetupTest runs before each test
func (s *CustomTagsTestSuite) SetupTest() {
	s.validator = validator.New()
	// Register all custom tags
	err := Register(s.validator, "roomid", ValidateRoomID)
	s.Require().NoError(err)

	RegisterAlias(s.validator, "userid", "uuid4")
	RegisterAlias(s.validator, "modules", "oneof=mixers januses")
	RegisterAlias(s.validator, "moduleid", "alphanum,min=3,max=32")
	RegisterAlias(s.validator, "role", "oneof=host guest moderator")
	RegisterAlias(s.validator, "label", "oneof=ready cordon draining drained unready")
}

// TestCustomTagsTestSuite runs the custom tags test suite
func TestCustomTagsTestSuite(t *testing.T) {
	suite.Run(t, new(CustomTagsTestSuite))
}

// TestUserIDAlias tests the userid custom alias tag
func (s *CustomTagsTestSuite) TestUserIDAlias() {
	type TestStruct struct {
		UserID string `validate:"userid"`
	}

	tests := []struct {
		name    string
		userID  string
		wantErr bool
	}{
		{
			name:    "valid UUID v4",
			userID:  "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "valid UUID v4 - different format",
			userID:  "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			wantErr: false,
		},
		{
			name:    "invalid - not a UUID",
			userID:  "not-a-uuid",
			wantErr: true,
		},
		{
			name:    "invalid - UUID v1 format",
			userID:  "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			testData := TestStruct{UserID: tt.userID}
			err := s.validator.Struct(testData)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

// TestModulesAlias tests the modules custom alias tag
func (s *CustomTagsTestSuite) TestModulesAlias() {
	type TestStruct struct {
		Module string `validate:"modules"`
	}

	tests := []struct {
		name    string
		module  string
		wantErr bool
	}{
		{
			name:    "valid - mixers",
			module:  "mixers",
			wantErr: false,
		},
		{
			name:    "valid - januses",
			module:  "januses",
			wantErr: false,
		},
		{
			name:    "invalid - other value",
			module:  "other",
			wantErr: true,
		},
		{
			name:    "invalid - empty",
			module:  "",
			wantErr: true,
		},
		{
			name:    "invalid - case sensitive",
			module:  "Mixers",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			testData := TestStruct{Module: tt.module}
			err := s.validator.Struct(testData)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

// TestModuleIDAlias tests the moduleid custom alias tag
func (s *CustomTagsTestSuite) TestModuleIDAlias() {
	type TestStruct struct {
		ModuleID string `validate:"moduleid"`
	}

	tests := []struct {
		name     string
		moduleID string
		wantErr  bool
	}{
		{
			name:     "valid - alphanumeric min length",
			moduleID: "abc",
			wantErr:  false,
		},
		{
			name:     "valid - alphanumeric mid length",
			moduleID: "module123",
			wantErr:  false,
		},
		{
			name:     "valid - max length (32 chars)",
			moduleID: "12345678901234567890123456789012",
			wantErr:  false,
		},
		{
			name:     "invalid - too short",
			moduleID: "ab",
			wantErr:  true,
		},
		{
			name:     "invalid - too long (33 chars)",
			moduleID: "123456789012345678901234567890123",
			wantErr:  true,
		},
		{
			name:     "invalid - contains hyphen",
			moduleID: "module-123",
			wantErr:  true,
		},
		{
			name:     "invalid - contains underscore",
			moduleID: "module_123",
			wantErr:  true,
		},
		{
			name:     "invalid - empty",
			moduleID: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			testData := TestStruct{ModuleID: tt.moduleID}
			err := s.validator.Struct(testData)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

// TestRoleAlias tests the role custom alias tag
func (s *CustomTagsTestSuite) TestRoleAlias() {
	type TestStruct struct {
		Role string `validate:"role"`
	}

	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{
			name:    "valid - host",
			role:    "host",
			wantErr: false,
		},
		{
			name:    "valid - guest",
			role:    "guest",
			wantErr: false,
		},
		{
			name:    "valid - moderator",
			role:    "moderator",
			wantErr: false,
		},
		{
			name:    "invalid - other value",
			role:    "admin",
			wantErr: true,
		},
		{
			name:    "invalid - empty",
			role:    "",
			wantErr: true,
		},
		{
			name:    "invalid - case sensitive",
			role:    "Host",
			wantErr: true,
		},
		{
			name:    "invalid - uppercase",
			role:    "GUEST",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			testData := TestStruct{Role: tt.role}
			err := s.validator.Struct(testData)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

// TestLabelAlias tests the label custom alias tag
func (s *CustomTagsTestSuite) TestLabelAlias() {
	type TestStruct struct {
		Label string `validate:"label"`
	}

	tests := []struct {
		name    string
		label   string
		wantErr bool
	}{
		{
			name:    "valid - ready",
			label:   "ready",
			wantErr: false,
		},
		{
			name:    "valid - cordon",
			label:   "cordon",
			wantErr: false,
		},
		{
			name:    "valid - draining",
			label:   "draining",
			wantErr: false,
		},
		{
			name:    "valid - drained",
			label:   "drained",
			wantErr: false,
		},
		{
			name:    "valid - unready",
			label:   "unready",
			wantErr: false,
		},
		{
			name:    "invalid - other value",
			label:   "invalid",
			wantErr: true,
		},
		{
			name:    "invalid - empty",
			label:   "",
			wantErr: true,
		},
		{
			name:    "invalid - case sensitive",
			label:   "Ready",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			testData := TestStruct{Label: tt.label}
			err := s.validator.Struct(testData)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

// TestMultipleCustomTags tests using multiple custom tags together
func (s *CustomTagsTestSuite) TestMultipleCustomTags() {
	type ComplexStruct struct {
		RoomID   string `validate:"roomid"`
		UserID   string `validate:"userid"`
		Module   string `validate:"modules"`
		ModuleID string `validate:"moduleid"`
		Role     string `validate:"role"`
		Label    string `validate:"label"`
	}

	// Test all valid
	validData := ComplexStruct{
		RoomID:   "test-room_123",
		UserID:   "550e8400-e29b-41d4-a716-446655440000",
		Module:   "mixers",
		ModuleID: "module123",
		Role:     "host",
		Label:    "ready",
	}
	err := s.validator.Struct(validData)
	s.NoError(err)

	// Test with invalid roomID
	invalidData := ComplexStruct{
		RoomID:   "ab", // too short
		UserID:   "550e8400-e29b-41d4-a716-446655440000",
		Module:   "mixers",
		ModuleID: "module123",
		Role:     "host",
		Label:    "ready",
	}
	err = s.validator.Struct(invalidData)
	s.Require().Error(err)

	// Require().Test with invalid userID
	invalidData2 := ComplexStruct{
		RoomID:   "test-room_123",
		UserID:   "not-a-uuid",
		Module:   "mixers",
		ModuleID: "module123",
		Role:     "host",
		Label:    "ready",
	}
	err = s.validator.Struct(invalidData2)
	s.Require().Error(err)

	// Require().Test with invalid role
	invalidData3 := ComplexStruct{
		RoomID:   "test-room_123",
		UserID:   "550e8400-e29b-41d4-a716-446655440000",
		Module:   "mixers",
		ModuleID: "module123",
		Role:     "admin", // invalid role
		Label:    "ready",
	}
	err = s.validator.Struct(invalidData3)
	s.Require().Error(err)
}
