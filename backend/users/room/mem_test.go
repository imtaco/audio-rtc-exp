package room

import (
	"testing"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/zset"
	"github.com/imtaco/audio-rtc-exp/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMemState() *roomsStateMem {
	return &roomsStateMem{
		rooms:      make(map[string]map[string]*users.User),
		userTracks: zset.New[string](),
		roomTracks: zset.New[string](),
		logger:     log.NewNop(),
	}
}

func TestRoomsStateMem_CreateRoomUser(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*roomsStateMem)
		roomID   string
		userID   string
		user     *users.User
		wantOk   bool
		validate func(*testing.T, *roomsStateMem)
	}{
		{
			name:   "create new user in new room",
			setup:  func(r *roomsStateMem) {},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Role: "anchor",
				Gen:  0,
				TS:   time.Now(),
			},
			wantOk: true,
			validate: func(t *testing.T, r *roomsStateMem) {
				assert.Contains(t, r.rooms, "room1")
				assert.Contains(t, r.rooms["room1"], "user1")
				assert.Equal(t, "anchor", r.rooms["room1"]["user1"].Role)
				assert.Equal(t, int32(0), r.rooms["room1"]["user1"].Gen)
			},
		},
		{
			name: "create new user in existing room",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{Role: "anchor"}
			},
			roomID: "room1",
			userID: "user2",
			user: &users.User{
				Role: "viewer",
				Gen:  0,
				TS:   time.Now(),
			},
			wantOk: true,
			validate: func(t *testing.T, r *roomsStateMem) {
				assert.Contains(t, r.rooms["room1"], "user2")
				assert.Equal(t, "viewer", r.rooms["room1"]["user2"].Role)
			},
		},
		{
			name: "create duplicate user should fail",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{Role: "anchor"}
			},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Role: "viewer",
				Gen:  0,
				TS:   time.Now(),
			},
			wantOk: false,
			validate: func(t *testing.T, r *roomsStateMem) {
				assert.Equal(t, "anchor", r.rooms["room1"]["user1"].Role)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestMemState()
			tt.setup(r)

			ok := r.createRoomUser(tt.roomID, tt.userID, tt.user)
			assert.Equal(t, tt.wantOk, ok)

			if tt.validate != nil {
				tt.validate(t, r)
			}
		})
	}
}

func TestRoomsStateMem_SetUserStatus(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		setup    func(*roomsStateMem)
		roomID   string
		userID   string
		user     *users.User
		wantOk   bool
		validate func(*testing.T, *roomsStateMem)
	}{
		{
			name: "set status for existing user",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{
					Role: "anchor",
					Gen:  0,
				}
			},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Status: constants.AnchorStatusOnAir,
				Gen:    1,
				TS:     now,
			},
			wantOk: true,
			validate: func(t *testing.T, r *roomsStateMem) {
				assert.Equal(t, constants.AnchorStatusOnAir, r.rooms["room1"]["user1"].Status)
				assert.Equal(t, int32(1), r.rooms["room1"]["user1"].Gen)
			},
		},
		{
			name: "clear status sets empty status",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{
					Role:   "anchor",
					Status: constants.AnchorStatusOnAir,
					Gen:    0,
				}
				r.userTracks.Put("user1", "room1", now.Add(-time.Minute))
			},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Status: "",
				Gen:    1,
				TS:     now,
			},
			wantOk: true,
			validate: func(t *testing.T, r *roomsStateMem) {
				assert.Equal(t, constants.AnchorStatus(""), r.rooms["room1"]["user1"].Status)
				assert.Equal(t, int32(1), r.rooms["room1"]["user1"].Gen)
			},
		},
		// TODO: re-enable after gen design is finalized
		// {
		// 	name: "reject older generation",
		// 	setup: func(r *roomsStateMem) {
		// 		r.rooms["room1"] = make(map[string]*users.User)
		// 		r.rooms["room1"]["user1"] = &users.User{
		// 			Role:   "anchor",
		// 			Status: constants.AnchorStatusOnAir,
		// 			Gen:    5,
		// 		}
		// 	},
		// 	roomID: "room1",
		// 	userID: "user1",
		// 	user: &users.User{
		// 		Status: "offline",
		// 		Gen:    3,
		// 		TS:     now,
		// 	},
		// 	wantOk: false,
		// 	validate: func(t *testing.T, r *roomsStateMem) {
		// 		assert.Equal(t, constants.AnchorStatusOnAir, r.rooms["room1"]["user1"].Status)
		// 		assert.Equal(t, int32(5), r.rooms["room1"]["user1"].Gen)
		// 	},
		// },
		{
			name:   "set status for non-existent room",
			setup:  func(r *roomsStateMem) {},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Status: constants.AnchorStatusOnAir,
				Gen:    1,
				TS:     now,
			},
			wantOk:   false,
			validate: func(t *testing.T, r *roomsStateMem) {},
		},
		{
			name: "set status for non-existent user",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
			},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Status: constants.AnchorStatusOnAir,
				Gen:    1,
				TS:     now,
			},
			wantOk:   false,
			validate: func(t *testing.T, r *roomsStateMem) {},
		},
		{
			name: "set status for user without role",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{
					Role: "",
				}
			},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Status: constants.AnchorStatusOnAir,
				Gen:    1,
				TS:     now,
			},
			wantOk:   false,
			validate: func(t *testing.T, r *roomsStateMem) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestMemState()
			tt.setup(r)

			ok := r.setUserStatus(tt.roomID, tt.userID, tt.user)
			assert.Equal(t, tt.wantOk, ok)

			if tt.validate != nil {
				tt.validate(t, r)
			}
		})
	}
}

func TestRoomsStateMem_RemoveRoomUser(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*roomsStateMem)
		roomID       string
		userID       string
		wantOk       bool
		wantLastUser bool
		validate     func(*testing.T, *roomsStateMem)
	}{
		{
			name: "remove user from room with multiple users",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{Role: "anchor"}
				r.rooms["room1"]["user2"] = &users.User{Role: "viewer"}
				r.userTracks.Put("user1", "room1", time.Now())
			},
			roomID:       "room1",
			userID:       "user1",
			wantOk:       true,
			wantLastUser: false,
			validate: func(t *testing.T, r *roomsStateMem) {
				assert.NotContains(t, r.rooms["room1"], "user1")
				assert.Contains(t, r.rooms["room1"], "user2")
				assert.Contains(t, r.rooms, "room1")
			},
		},
		{
			name: "remove last user from room",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{Role: "anchor"}
				r.userTracks.Put("user1", "room1", time.Now())
				r.roomTracks.Put("room1", "", time.Now())
			},
			roomID:       "room1",
			userID:       "user1",
			wantOk:       true,
			wantLastUser: true,
			validate: func(t *testing.T, r *roomsStateMem) {
				assert.NotContains(t, r.rooms, "room1")
			},
		},
		{
			name:         "remove from non-existent room",
			setup:        func(r *roomsStateMem) {},
			roomID:       "room1",
			userID:       "user1",
			wantOk:       false,
			wantLastUser: false,
			validate:     func(t *testing.T, r *roomsStateMem) {},
		},
		{
			name: "remove non-existent user from room",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{Role: "anchor"}
			},
			roomID:       "room1",
			userID:       "user2",
			wantOk:       false,
			wantLastUser: false,
			validate: func(t *testing.T, r *roomsStateMem) {
				assert.Contains(t, r.rooms["room1"], "user1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestMemState()
			tt.setup(r)

			ok, lastUser := r.removeRoomUser(tt.roomID, tt.userID)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.wantLastUser, lastUser)

			if tt.validate != nil {
				tt.validate(t, r)
			}
		})
	}
}

func TestRoomsStateMem_GetRoomUsers(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*roomsStateMem)
		roomID   string
		validate func(*testing.T, map[string]users.User)
	}{
		{
			name: "get users from room with multiple users",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
				r.rooms["room1"]["user1"] = &users.User{
					Role:   "anchor",
					Status: constants.AnchorStatusOnAir,
					Gen:    1,
				}
				r.rooms["room1"]["user2"] = &users.User{
					Role:   "viewer",
					Status: constants.AnchorStatusLeft,
					Gen:    2,
				}
			},
			roomID: "room1",
			validate: func(t *testing.T, users map[string]users.User) {
				require.NotNil(t, users)
				assert.Len(t, users, 2)
				assert.Equal(t, "anchor", users["user1"].Role)
				assert.Equal(t, constants.AnchorStatusOnAir, users["user1"].Status)
				assert.Equal(t, "viewer", users["user2"].Role)
			},
		},
		{
			name:   "get users from non-existent room",
			setup:  func(r *roomsStateMem) {},
			roomID: "room1",
			validate: func(t *testing.T, users map[string]users.User) {
				assert.Nil(t, users)
			},
		},
		{
			name: "get users from empty room",
			setup: func(r *roomsStateMem) {
				r.rooms["room1"] = make(map[string]*users.User)
			},
			roomID: "room1",
			validate: func(t *testing.T, users map[string]users.User) {
				require.NotNil(t, users)
				assert.Len(t, users, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestMemState()
			tt.setup(r)

			users := r.getRoomUsers(tt.roomID)
			tt.validate(t, users)
		})
	}
}

func TestRoomsStateMem_AddRoomTrack(t *testing.T) {
	r := newTestMemState()
	now := time.Now()

	r.addRoomTrack("room1", now)

	roomID, _, ts, ok := r.roomTracks.Peek()
	assert.True(t, ok)
	assert.Equal(t, "room1", roomID)
	assert.Equal(t, now, ts)
}

func TestEnsureUser(t *testing.T) {
	us := make(map[string]*users.User)

	t.Run("create new user", func(t *testing.T) {
		u := ensureUser(us, "user1")
		require.NotNil(t, u)
		assert.Contains(t, us, "user1")
	})

	t.Run("get existing user", func(t *testing.T) {
		us["user2"] = &users.User{Role: "anchor"}
		u := ensureUser(us, "user2")
		require.NotNil(t, u)
		assert.Equal(t, "anchor", u.Role)
	})
}
