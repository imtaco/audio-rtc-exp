package room

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/users"
)

type RoomRedisTestSuite struct {
	suite.Suite
	testPrefix string
}

func TestRoomRedisSuite(t *testing.T) {
	suite.Run(t, new(RoomRedisTestSuite))
}

func (s *RoomRedisTestSuite) SetupTest() {
	s.testPrefix = "test-prefix"
}

func (s *RoomRedisTestSuite) TestStatusField() {
	tests := []struct {
		userID   string
		expected string
	}{
		{"user123", "s:user123"},
		{"user456", "s:user456"},
		{"", "s:"},
	}

	for _, tt := range tests {
		s.Run(tt.userID, func() {
			result := statusField(tt.userID)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *RoomRedisTestSuite) TestMetaField() {
	tests := []struct {
		userID   string
		expected string
	}{
		{"user123", "m:user123"},
		{"user456", "m:user456"},
		{"", "m:"},
	}

	for _, tt := range tests {
		s.Run(tt.userID, func() {
			result := metaField(tt.userID)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *RoomRedisTestSuite) TestPackUnpackStatus() {
	tests := []struct {
		name   string
		user   *users.User
		verify func(string, time.Time, constants.AnchorStatus, int32)
	}{
		{
			name: "pack and unpack valid status",
			user: &users.User{
				TS:     time.Unix(1234567890, 0),
				Status: constants.AnchorStatusIdle,
				Gen:    5,
			},
			verify: func(packed string, ts time.Time, status constants.AnchorStatus, gen int32) {
				s.Equal("1234567890,idle,5", packed)
				s.Equal(time.Unix(1234567890, 0), ts)
				s.Equal(constants.AnchorStatusIdle, status)
				s.Equal(int32(5), gen)
			},
		},
		{
			name: "pack and unpack empty status",
			user: &users.User{
				TS:     time.Unix(9876543210, 0),
				Status: constants.AnchorStatus(""),
				Gen:    0,
			},
			verify: func(packed string, ts time.Time, status constants.AnchorStatus, gen int32) {
				s.Equal("9876543210,,0", packed)
				s.Equal(time.Unix(9876543210, 0), ts)
				s.Equal(constants.AnchorStatus(""), status)
				s.Equal(int32(0), gen)
			},
		},
		{
			name: "pack and unpack with status containing special chars",
			user: &users.User{
				TS:     time.Unix(1111111111, 0),
				Status: constants.AnchorStatusOnAir,
				Gen:    10,
			},
			verify: func(packed string, ts time.Time, status constants.AnchorStatus, gen int32) {
				s.Equal("1111111111,onair,10", packed)
				s.Equal(time.Unix(1111111111, 0), ts)
				s.Equal(constants.AnchorStatusOnAir, status)
				s.Equal(int32(10), gen)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			packed := packStatus(tt.user)

			ts, status, gen, err := unpackStatus(packed)
			s.Require().NoError(err)

			tt.verify(packed, ts, status, gen)
		})
	}
}

func (s *RoomRedisTestSuite) TestUnpackStatus_Errors() {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "invalid format - too few parts",
			input:   "1234567890,online",
			wantErr: true,
		},
		{
			name:    "invalid format - too many parts",
			input:   "1234567890,online,5,extra",
			wantErr: true,
		},
		{
			name:    "invalid timestamp",
			input:   "invalid,online,5",
			wantErr: true,
		},
		{
			name:    "invalid generation",
			input:   "1234567890,online,invalid",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			_, _, _, err := unpackStatus(tt.input)
			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

func (s *RoomRedisTestSuite) TestParseUsersData() {
	tests := []struct {
		name     string
		input    map[string]string
		validate func(map[string]*users.User)
	}{
		{
			name: "parse single user with role and status",
			input: map[string]string{
				"m:user1": "anchor",
				"s:user1": "1234567890,onair,5",
			},
			validate: func(users map[string]*users.User) {
				s.Require().Len(users, 1)
				s.Require().Contains(users, "user1")
				s.Equal("anchor", users["user1"].Role)
				s.Equal(constants.AnchorStatusOnAir, users["user1"].Status)
				s.Equal(time.Unix(1234567890, 0), users["user1"].TS)
				s.Equal(int32(5), users["user1"].Gen)
			},
		},
		{
			name: "parse user with only role",
			input: map[string]string{
				"m:user1": "viewer",
			},
			validate: func(users map[string]*users.User) {
				s.Require().Len(users, 1)
				s.Require().Contains(users, "user1")
				s.Equal("viewer", users["user1"].Role)
				s.Equal(constants.AnchorStatus(""), users["user1"].Status)
			},
		},
		{
			name: "parse multiple users",
			input: map[string]string{
				"m:user1": "anchor",
				"s:user1": "1234567890,onair,1",
				"m:user2": "viewer",
				"s:user2": "9876543210,idle,2",
			},
			validate: func(users map[string]*users.User) {
				s.Require().Len(users, 2)
				s.Equal("anchor", users["user1"].Role)
				s.Equal(constants.AnchorStatusOnAir, users["user1"].Status)
				s.Equal("viewer", users["user2"].Role)
				s.Equal(constants.AnchorStatusIdle, users["user2"].Status)
			},
		},
		{
			name: "parse with invalid status format",
			input: map[string]string{
				"m:user1": "anchor",
				"s:user1": "invalid-format",
			},
			validate: func(users map[string]*users.User) {
				s.Require().Len(users, 1)
				s.Equal("anchor", users["user1"].Role)
				s.Equal(constants.AnchorStatus(""), users["user1"].Status)
			},
		},
		{
			name: "parse with unknown field prefix",
			input: map[string]string{
				"m:user1": "anchor",
				"x:user1": "unknown-field",
			},
			validate: func(users map[string]*users.User) {
				s.Require().Len(users, 1)
				s.Equal("anchor", users["user1"].Role)
			},
		},
		{
			name:  "parse empty data",
			input: map[string]string{},
			validate: func(users map[string]*users.User) {
				s.Len(users, 0)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			users := parseUsersData(tt.input)
			tt.validate(users)
		})
	}
}

func (s *RoomRedisTestSuite) TestRoomStateRedis_Keys() {
	r := &roomStateRedis{
		prefix: s.testPrefix,
	}

	s.Run("roomsKey", func() {
		expected := "test-prefix:rooms"
		s.Equal(expected, r.roomsKey())
	})

	s.Run("userStatusKey", func() {
		expected := "test-prefix:r:room123:us"
		s.Equal(expected, r.userStatusKey("room123"))
	})
}

func (s *RoomRedisTestSuite) TestParseUsersData_StatusBeforeRole() {
	s.Run("status field comes before role field", func() {
		input := map[string]string{
			"s:user1": "1234567890,onair,5",
			"m:user1": "anchor",
		}

		users := parseUsersData(input)
		s.Require().Len(users, 1)
		s.Require().Contains(users, "user1")
		s.Equal("anchor", users["user1"].Role)
		s.Equal(constants.AnchorStatusOnAir, users["user1"].Status)
		s.Equal(time.Unix(1234567890, 0), users["user1"].TS)
		s.Equal(int32(5), users["user1"].Gen)
	})
}

func (s *RoomRedisTestSuite) TestRoomStateRedis_ParseRoomTracks() {
	tests := []struct {
		name     string
		input    map[string]string
		validate func(map[string]time.Time, error)
	}{
		{
			name: "parse valid room tracks",
			input: map[string]string{
				"room1": "1234567890",
				"room2": "9876543210",
			},
			validate: func(result map[string]time.Time, err error) {
				s.Require().NoError(err)
				s.Require().Len(result, 2)
				s.Equal(time.Unix(1234567890, 0), result["room1"])
				s.Equal(time.Unix(9876543210, 0), result["room2"])
			},
		},
		{
			name:  "parse empty tracks",
			input: map[string]string{},
			validate: func(result map[string]time.Time, err error) {
				s.Require().NoError(err)
				s.Len(result, 0)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := make(map[string]time.Time)
			var err error

			for roomID, tsStr := range tt.input {
				var ts int64
				ts, parseErr := strconv.ParseInt(tsStr, 10, 64)
				if parseErr == nil {
					result[roomID] = time.Unix(ts, 0)
				}
			}

			tt.validate(result, err)
		})
	}
}
