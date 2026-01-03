package room

import (
	"strconv"
	"testing"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusField(t *testing.T) {
	tests := []struct {
		userID   string
		expected string
	}{
		{"user123", "s:user123"},
		{"user456", "s:user456"},
		{"", "s:"},
	}

	for _, tt := range tests {
		t.Run(tt.userID, func(t *testing.T) {
			result := statusField(tt.userID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetaField(t *testing.T) {
	tests := []struct {
		userID   string
		expected string
	}{
		{"user123", "m:user123"},
		{"user456", "m:user456"},
		{"", "m:"},
	}

	for _, tt := range tests {
		t.Run(tt.userID, func(t *testing.T) {
			result := metaField(tt.userID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPackUnpackStatus(t *testing.T) {
	tests := []struct {
		name   string
		user   *users.User
		verify func(*testing.T, string, time.Time, constants.AnchorStatus, int32)
	}{
		{
			name: "pack and unpack valid status",
			user: &users.User{
				TS:     time.Unix(1234567890, 0),
				Status: constants.AnchorStatusIdle,
				Gen:    5,
			},
			verify: func(t *testing.T, packed string, ts time.Time, status constants.AnchorStatus, gen int32) {
				assert.Equal(t, "1234567890,idle,5", packed)
				assert.Equal(t, time.Unix(1234567890, 0), ts)
				assert.Equal(t, constants.AnchorStatusIdle, status)
				assert.Equal(t, int32(5), gen)
			},
		},
		{
			name: "pack and unpack empty status",
			user: &users.User{
				TS:     time.Unix(9876543210, 0),
				Status: constants.AnchorStatus(""),
				Gen:    0,
			},
			verify: func(t *testing.T, packed string, ts time.Time, status constants.AnchorStatus, gen int32) {
				assert.Equal(t, "9876543210,,0", packed)
				assert.Equal(t, time.Unix(9876543210, 0), ts)
				assert.Equal(t, constants.AnchorStatus(""), status)
				assert.Equal(t, int32(0), gen)
			},
		},
		{
			name: "pack and unpack with status containing special chars",
			user: &users.User{
				TS:     time.Unix(1111111111, 0),
				Status: constants.AnchorStatusOnAir,
				Gen:    10,
			},
			verify: func(t *testing.T, packed string, ts time.Time, status constants.AnchorStatus, gen int32) {
				assert.Equal(t, "1111111111,onair,10", packed)
				assert.Equal(t, time.Unix(1111111111, 0), ts)
				assert.Equal(t, constants.AnchorStatusOnAir, status)
				assert.Equal(t, int32(10), gen)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packed := packStatus(tt.user)

			ts, status, gen, err := unpackStatus(packed)
			require.NoError(t, err)

			tt.verify(t, packed, ts, status, gen)
		})
	}
}

func TestUnpackStatus_Errors(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := unpackStatus(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseUsersData(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		validate func(*testing.T, map[string]*users.User)
	}{
		{
			name: "parse single user with role and status",
			input: map[string]string{
				"m:user1": "anchor",
				"s:user1": "1234567890,onair,5",
			},
			validate: func(t *testing.T, users map[string]*users.User) {
				require.Len(t, users, 1)
				require.Contains(t, users, "user1")
				assert.Equal(t, "anchor", users["user1"].Role)
				assert.Equal(t, constants.AnchorStatusOnAir, users["user1"].Status)
				assert.Equal(t, time.Unix(1234567890, 0), users["user1"].TS)
				assert.Equal(t, int32(5), users["user1"].Gen)
			},
		},
		{
			name: "parse user with only role",
			input: map[string]string{
				"m:user1": "viewer",
			},
			validate: func(t *testing.T, users map[string]*users.User) {
				require.Len(t, users, 1)
				require.Contains(t, users, "user1")
				assert.Equal(t, "viewer", users["user1"].Role)
				assert.Equal(t, constants.AnchorStatus(""), users["user1"].Status)
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
			validate: func(t *testing.T, users map[string]*users.User) {
				require.Len(t, users, 2)
				assert.Equal(t, "anchor", users["user1"].Role)
				assert.Equal(t, constants.AnchorStatusOnAir, users["user1"].Status)
				assert.Equal(t, "viewer", users["user2"].Role)
				assert.Equal(t, constants.AnchorStatusIdle, users["user2"].Status)
			},
		},
		{
			name: "parse with invalid status format",
			input: map[string]string{
				"m:user1": "anchor",
				"s:user1": "invalid-format",
			},
			validate: func(t *testing.T, users map[string]*users.User) {
				require.Len(t, users, 1)
				assert.Equal(t, "anchor", users["user1"].Role)
				assert.Equal(t, constants.AnchorStatus(""), users["user1"].Status)
			},
		},
		{
			name: "parse with unknown field prefix",
			input: map[string]string{
				"m:user1": "anchor",
				"x:user1": "unknown-field",
			},
			validate: func(t *testing.T, users map[string]*users.User) {
				require.Len(t, users, 1)
				assert.Equal(t, "anchor", users["user1"].Role)
			},
		},
		{
			name:  "parse empty data",
			input: map[string]string{},
			validate: func(t *testing.T, users map[string]*users.User) {
				assert.Len(t, users, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := parseUsersData(tt.input)
			tt.validate(t, users)
		})
	}
}

func TestRoomStateRedis_Keys(t *testing.T) {
	r := &roomStateRedis{
		prefix: "test-prefix",
	}

	t.Run("roomsKey", func(t *testing.T) {
		expected := "test-prefix:rooms"
		assert.Equal(t, expected, r.roomsKey())
	})

	t.Run("userStatusKey", func(t *testing.T) {
		expected := "test-prefix:r:room123:us"
		assert.Equal(t, expected, r.userStatusKey("room123"))
	})
}

func TestParseUsersData_StatusBeforeRole(t *testing.T) {
	t.Run("status field comes before role field", func(t *testing.T) {
		input := map[string]string{
			"s:user1": "1234567890,onair,5",
			"m:user1": "anchor",
		}

		users := parseUsersData(input)
		require.Len(t, users, 1)
		require.Contains(t, users, "user1")
		assert.Equal(t, "anchor", users["user1"].Role)
		assert.Equal(t, constants.AnchorStatusOnAir, users["user1"].Status)
		assert.Equal(t, time.Unix(1234567890, 0), users["user1"].TS)
		assert.Equal(t, int32(5), users["user1"].Gen)
	})
}

func TestRoomStateRedis_ParseRoomTracks(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		validate func(*testing.T, map[string]time.Time, error)
	}{
		{
			name: "parse valid room tracks",
			input: map[string]string{
				"room1": "1234567890",
				"room2": "9876543210",
			},
			validate: func(t *testing.T, result map[string]time.Time, err error) {
				require.NoError(t, err)
				require.Len(t, result, 2)
				assert.Equal(t, time.Unix(1234567890, 0), result["room1"])
				assert.Equal(t, time.Unix(9876543210, 0), result["room2"])
			},
		},
		{
			name:  "parse empty tracks",
			input: map[string]string{},
			validate: func(t *testing.T, result map[string]time.Time, err error) {
				require.NoError(t, err)
				assert.Len(t, result, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]time.Time)
			var err error

			for roomID, tsStr := range tt.input {
				var ts int64
				ts, parseErr := strconv.ParseInt(tsStr, 10, 64)
				if parseErr == nil {
					result[roomID] = time.Unix(ts, 0)
				}
			}

			tt.validate(t, result, err)
		})
	}
}
