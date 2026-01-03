package log

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zapcore"
)

// ModuleLevelTestSuite tests the moduleLevel function
type ModuleLevelTestSuite struct {
	suite.Suite
	originalEnvFunc func(string) (string, bool)
	testEnv         map[string]string
}

func (s *ModuleLevelTestSuite) SetupTest() {
	// Save original envFunc
	s.originalEnvFunc = envFunc
	s.testEnv = make(map[string]string)

	// Replace envFunc with test implementation that mimics env() behavior
	envFunc = func(key string) (string, bool) {
		val, ok := s.testEnv[key]
		if !ok {
			return "", false
		}
		// Mimic the env() function's behavior: trim and check if empty
		trimmed := ""
		for _, c := range val {
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				trimmed = val
				break
			}
		}
		if trimmed != "" {
			// Simple trim - remove leading/trailing spaces
			start := 0
			end := len(val)
			for start < end && (val[start] == ' ' || val[start] == '\t' || val[start] == '\n' || val[start] == '\r') {
				start++
			}
			for end > start && (val[end-1] == ' ' || val[end-1] == '\t' || val[end-1] == '\n' || val[end-1] == '\r') {
				end--
			}
			trimmed = val[start:end]
		}
		if trimmed == "" {
			return "", false
		}
		return trimmed, true
	}
}

func (s *ModuleLevelTestSuite) TearDownTest() {
	// Restore original envFunc
	envFunc = s.originalEnvFunc
	s.testEnv = nil
}

func (s *ModuleLevelTestSuite) setEnv(key, value string) {
	s.testEnv[key] = value
}

func (s *ModuleLevelTestSuite) setEnvVars(envVars map[string]string) {
	for k, v := range envVars {
		s.testEnv[k] = v
	}
}

func (s *ModuleLevelTestSuite) TestNoEnvVars_DefaultsToInfo() {
	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.InfoLevel, level)
}

func (s *ModuleLevelTestSuite) TestGlobalLogLevelOnly() {
	s.setEnv("LOG_LEVEL", "debug")

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestSingleLevelModule_SpecificOverride() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL":           "warn",
		"LOG_LEVEL__ROOM_SVC": "debug",
	})

	level := moduleLevel([]string{"RoomSvc"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestSingleLevelModule_UsesGlobalWhenNoSpecific() {
	s.setEnv("LOG_LEVEL", "error")

	level := moduleLevel([]string{"RoomSvc"})
	s.Equal(zapcore.ErrorLevel, level)
}

func (s *ModuleLevelTestSuite) TestTwoLevelModule_MostSpecificWins() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL":                             "warn",
		"LOG_LEVEL__RESOURCE_MGR":               "info",
		"LOG_LEVEL__RESOURCE_MGR__JANUS_WORKER": "debug",
	})

	level := moduleLevel([]string{"ResourceMgr", "JanusWorker"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestTwoLevelModule_InheritsParentLevel() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL":               "warn",
		"LOG_LEVEL__RESOURCE_MGR": "debug",
	})

	level := moduleLevel([]string{"ResourceMgr", "JanusWorker"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestTwoLevelModule_FallsBackToGlobal() {
	s.setEnv("LOG_LEVEL", "error")

	level := moduleLevel([]string{"ResourceMgr", "JanusWorker"})
	s.Equal(zapcore.ErrorLevel, level)
}

func (s *ModuleLevelTestSuite) TestThreeLevelModule_MostSpecificWins() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL":                                    "warn",
		"LOG_LEVEL__SERVICE":                           "info",
		"LOG_LEVEL__SERVICE__COMPONENT":                "debug",
		"LOG_LEVEL__SERVICE__COMPONENT__SUB_COMPONENT": "error",
	})

	level := moduleLevel([]string{"Service", "Component", "SubComponent"})
	s.Equal(zapcore.ErrorLevel, level)
}

func (s *ModuleLevelTestSuite) TestThreeLevelModule_InheritsMiddleLevel() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL":                     "warn",
		"LOG_LEVEL__SERVICE":            "info",
		"LOG_LEVEL__SERVICE__COMPONENT": "debug",
	})

	level := moduleLevel([]string{"Service", "Component", "SubComponent"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestThreeLevelModule_InheritsTopLevel() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL":          "warn",
		"LOG_LEVEL__SERVICE": "debug",
	})

	level := moduleLevel([]string{"Service", "Component", "SubComponent"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestCamelCaseConvertedToScreamingSnakeCase() {
	s.setEnv("LOG_LEVEL__MY_TEST_MODULE", "debug")

	level := moduleLevel([]string{"MyTestModule"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestMixedCaseNamesConvertedProperly() {
	s.setEnv("LOG_LEVEL__HTTP_SERVER__WEB_SOCKET_HANDLER", "debug")

	level := moduleLevel([]string{"HTTPServer", "WebSocketHandler"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestInvalidLevelIgnored_FallsBackToNextPriority() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL__TEST_MODULE": "invalid",
		"LOG_LEVEL":              "warn",
	})

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.WarnLevel, level)
}

func (s *ModuleLevelTestSuite) TestAllLevels_Fatal() {
	s.setEnv("LOG_LEVEL__TEST_MODULE", "fatal")

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.FatalLevel, level)
}

func (s *ModuleLevelTestSuite) TestAllLevels_Error() {
	s.setEnv("LOG_LEVEL__TEST_MODULE", "error")

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.ErrorLevel, level)
}

func (s *ModuleLevelTestSuite) TestAllLevels_Warn() {
	s.setEnv("LOG_LEVEL__TEST_MODULE", "warn")

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.WarnLevel, level)
}

func (s *ModuleLevelTestSuite) TestAllLevels_Info() {
	s.setEnv("LOG_LEVEL__TEST_MODULE", "info")

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.InfoLevel, level)
}

func (s *ModuleLevelTestSuite) TestAllLevels_Debug() {
	s.setEnv("LOG_LEVEL__TEST_MODULE", "debug")

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestCaseInsensitiveLevelParsing() {
	s.setEnv("LOG_LEVEL__TEST_MODULE", "DEBUG")

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestWhitespaceTrimmedFromEnvValues() {
	s.setEnv("LOG_LEVEL__TEST_MODULE", "  debug  ")

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestEmptyStringIgnored() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL__TEST_MODULE": "",
		"LOG_LEVEL":              "warn",
	})

	level := moduleLevel([]string{"TestModule"})
	s.Equal(zapcore.WarnLevel, level)
}

func (s *ModuleLevelTestSuite) TestPriorityOrder_SpecificOverParent() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL":                "fatal",
		"LOG_LEVEL__PARENT":        "error",
		"LOG_LEVEL__PARENT__CHILD": "debug",
	})

	level := moduleLevel([]string{"Parent", "Child"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestEmptyModuleNames() {
	level := moduleLevel([]string{})
	s.Equal(zapcore.InfoLevel, level)
}

func (s *ModuleLevelTestSuite) TestNilModuleNames() {
	level := moduleLevel(nil)
	s.Equal(zapcore.InfoLevel, level)
}

func (s *ModuleLevelTestSuite) TestComplexHierarchy() {
	s.setEnv("LOG_LEVEL__A__B__C__D__E", "debug")

	level := moduleLevel([]string{"A", "B", "C", "D", "E"})
	s.Equal(zapcore.DebugLevel, level)
}

func (s *ModuleLevelTestSuite) TestPartialHierarchyMatch() {
	s.setEnvVars(map[string]string{
		"LOG_LEVEL":       "error",
		"LOG_LEVEL__A":    "warn",
		"LOG_LEVEL__A__B": "info",
		// No LOG_LEVEL__A__B__C set
	})

	level := moduleLevel([]string{"A", "B", "C"})
	s.Equal(zapcore.InfoLevel, level, "Should inherit from parent LOG_LEVEL__A__B")
}

func TestModuleLevelTestSuite(t *testing.T) {
	suite.Run(t, new(ModuleLevelTestSuite))
}

// ParseLevelTestSuite tests the parseLevel function
type ParseLevelTestSuite struct {
	suite.Suite
}

func (s *ParseLevelTestSuite) TestValidLevels() {
	tests := []struct {
		input     string
		wantLevel zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
		{"error", zapcore.ErrorLevel},
		{"fatal", zapcore.FatalLevel},
		{"DEBUG", zapcore.DebugLevel},
		{"DeBuG", zapcore.DebugLevel},
		{"", zapcore.InfoLevel}, // Empty string is parsed as info by zap
	}

	for _, tt := range tests {
		s.Run(tt.input, func() {
			level, ok := parseLevel(tt.input)
			s.True(ok, "Expected parseLevel(%q) to return ok=true", tt.input)
			s.Equal(tt.wantLevel, level, "Expected parseLevel(%q) to return %v", tt.input, tt.wantLevel)
		})
	}
}

func (s *ParseLevelTestSuite) TestInvalidLevels() {
	tests := []string{"invalid", "random", "trace"}

	for _, input := range tests {
		s.Run(input, func() {
			_, ok := parseLevel(input)
			s.False(ok, "Expected parseLevel(%q) to return ok=false", input)
		})
	}
}

func TestParseLevelTestSuite(t *testing.T) {
	suite.Run(t, new(ParseLevelTestSuite))
}

// EnvTestSuite tests the env function
type EnvTestSuite struct {
	suite.Suite
	originalEnvFunc func(string) (string, bool)
}

func (s *EnvTestSuite) SetupTest() {
	// Save original envFunc
	s.originalEnvFunc = envFunc
	// Restore to use real env function for testing env()
	envFunc = env
}

func (s *EnvTestSuite) TearDownTest() {
	// Restore original envFunc
	envFunc = s.originalEnvFunc
}

func TestEnvTestSuite(t *testing.T) {
	suite.Run(t, new(EnvTestSuite))
}
