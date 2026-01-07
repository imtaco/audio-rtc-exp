package room

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/zset"
	"github.com/imtaco/audio-rtc-exp/users"
)

type CombinedRoomTestSuite struct {
	suite.Suite
	room        *combinedRoom
	redisClient *redis.Client
	mr          *miniredis.Miniredis
	ctx         context.Context
}

func (s *CombinedRoomTestSuite) SetupTest() {
	logger := log.NewNop()

	// Create miniredis instance
	mr, err := miniredis.Run()
	s.Require().NoError(err)

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	room := New(redisClient, "test", logger).(*combinedRoom)

	s.room = room
	s.redisClient = redisClient
	s.mr = mr
	s.ctx = context.Background()
}

func (s *CombinedRoomTestSuite) TearDownTest() {
	if s.redisClient != nil {
		s.redisClient.Close()
	}
	if s.mr != nil {
		s.mr.Close()
	}
}

func (s *CombinedRoomTestSuite) resetRoomState() {
	s.room.memState.rooms = make(map[string]map[string]*users.User)
	s.room.memState.userTracks = zset.New[string]()
	s.room.memState.roomTracks = zset.New[string]()
}

func TestCombinedRoomSuite(t *testing.T) {
	suite.Run(t, new(CombinedRoomTestSuite))
}

func (s *CombinedRoomTestSuite) TestCreateUser() {
	now := time.Now()

	tests := []struct {
		name     string
		setup    func()
		roomID   string
		userID   string
		user     *users.User
		wantOk   bool
		wantErr  bool
		validate func()
	}{
		{
			name:   "create new user successfully",
			setup:  func() {},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Role: "anchor",
				Gen:  0,
				TS:   now,
			},
			wantOk:  true,
			wantErr: false,
			validate: func() {
				users := s.room.GetRoomUsers(s.ctx, "room1")
				s.Require().Contains(users, "user1")
				s.Equal("anchor", users["user1"].Role)
			},
		},
		{
			name: "create duplicate user should fail",
			setup: func() {
				_, _ = s.room.CreateUser(s.ctx, "room2", "user1", &users.User{
					Role: "anchor",
					Gen:  0,
					TS:   now,
				})
			},
			roomID: "room2",
			userID: "user1",
			user: &users.User{
				Role: "viewer",
				Gen:  0,
				TS:   now,
			},
			wantOk:  false,
			wantErr: false,
			validate: func() {
				users := s.room.GetRoomUsers(s.ctx, "room2")
				s.Equal("anchor", users["user1"].Role)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.resetRoomState()

			tt.setup()

			ok, err := s.room.CreateUser(s.ctx, tt.roomID, tt.userID, tt.user)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}

			s.Equal(tt.wantOk, ok)

			if tt.validate != nil {
				tt.validate()
			}
		})
	}
}

func (s *CombinedRoomTestSuite) TestUpdateUserStatus() {
	now := time.Now()

	tests := []struct {
		name     string
		setup    func()
		roomID   string
		userID   string
		user     *users.User
		wantOk   bool
		wantErr  bool
		validate func()
	}{
		{
			name: "update existing user status",
			setup: func() {
				_, _ = s.room.CreateUser(s.ctx, "room1", "user1", &users.User{
					Role: "anchor",
					Gen:  0,
					TS:   now,
				})
			},
			roomID: "room1",
			userID: "user1",
			user: &users.User{
				Status: constants.AnchorStatusOnAir,
				Gen:    1,
				TS:     now,
			},
			wantOk:  true,
			wantErr: false,
			validate: func() {
				users := s.room.GetRoomUsers(s.ctx, "room1")
				s.Equal(constants.AnchorStatusOnAir, users["user1"].Status)
				s.Equal(int32(1), users["user1"].Gen)
			},
		},
		{
			name:   "update non-existent user should fail",
			setup:  func() {},
			roomID: "room1",
			userID: "user999",
			user: &users.User{
				Status: constants.AnchorStatusOnAir,
				Gen:    1,
				TS:     now,
			},
			wantOk:  false,
			wantErr: false,
			validate: func() {
				users := s.room.GetRoomUsers(s.ctx, "room1")
				s.NotContains(users, "user999")
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.resetRoomState()

			tt.setup()

			ok, err := s.room.UpdateUserStatus(s.ctx, tt.roomID, tt.userID, tt.user)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}

			s.Equal(tt.wantOk, ok)

			if tt.validate != nil {
				tt.validate()
			}
		})
	}
}

func (s *CombinedRoomTestSuite) TestRemoveUser() {
	now := time.Now()

	tests := []struct {
		name     string
		setup    func()
		roomID   string
		userID   string
		wantOk   bool
		wantErr  bool
		validate func()
	}{
		{
			name: "remove existing user",
			setup: func() {
				_, _ = s.room.CreateUser(s.ctx, "room1", "user1", &users.User{
					Role: "anchor",
					Gen:  0,
					TS:   now,
				})
				_, _ = s.room.CreateUser(s.ctx, "room1", "user2", &users.User{
					Role: "viewer",
					Gen:  0,
					TS:   now,
				})
			},
			roomID:  "room1",
			userID:  "user1",
			wantOk:  true,
			wantErr: false,
			validate: func() {
				users := s.room.GetRoomUsers(s.ctx, "room1")
				s.NotContains(users, "user1")
				s.Contains(users, "user2")
			},
		},
		{
			name: "remove last user from room",
			setup: func() {
				_, _ = s.room.CreateUser(s.ctx, "room2", "user1", &users.User{
					Role: "anchor",
					Gen:  0,
					TS:   now,
				})
			},
			roomID:  "room2",
			userID:  "user1",
			wantOk:  true,
			wantErr: false,
			validate: func() {
				users := s.room.GetRoomUsers(s.ctx, "room2")
				s.Nil(users)
			},
		},
		{
			name:     "remove non-existent user",
			setup:    func() {},
			roomID:   "room1",
			userID:   "user999",
			wantOk:   false,
			wantErr:  false,
			validate: func() {},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.resetRoomState()

			tt.setup()

			ok, err := s.room.RemoveUser(s.ctx, tt.roomID, tt.userID)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}

			s.Equal(tt.wantOk, ok)

			if tt.validate != nil {
				tt.validate()
			}
		})
	}
}

func (s *CombinedRoomTestSuite) TestGetRoomUsers() {
	now := time.Now()

	s.Run("get users from existing room", func() {
		s.resetRoomState()

		_, _ = s.room.CreateUser(s.ctx, "room1", "user1", &users.User{
			Role: "anchor",
			Gen:  0,
			TS:   now,
		})
		_, _ = s.room.CreateUser(s.ctx, "room1", "user2", &users.User{
			Role: "viewer",
			Gen:  0,
			TS:   now,
		})

		users := s.room.GetRoomUsers(s.ctx, "room1")
		s.Require().NotNil(users)
		s.Len(users, 2)
		s.Contains(users, "user1")
		s.Contains(users, "user2")
	})

	s.Run("get users from non-existent room", func() {
		s.resetRoomState()

		users := s.room.GetRoomUsers(s.ctx, "room999")
		s.Nil(users)
	})
}

func (s *CombinedRoomTestSuite) TestRebuild() {
	now := time.Now()

	s.Run("rebuild from empty redis", func() {
		s.resetRoomState()

		err := s.room.Rebuild(s.ctx)
		s.Require().NoError(err)
		s.Len(s.room.memState.rooms, 0)
	})

	s.Run("rebuild from existing data", func() {
		s.resetRoomState()

		_, _ = s.room.CreateUser(s.ctx, "room1", "user1", &users.User{
			Role: "anchor",
			Gen:  0,
			TS:   now,
		})
		_, _ = s.room.UpdateUserStatus(s.ctx, "room1", "user1", &users.User{
			Status: constants.AnchorStatusOnAir,
			Gen:    1,
			TS:     now,
		})

		s.resetRoomState()

		err := s.room.Rebuild(s.ctx)
		s.Require().NoError(err)

		users := s.room.GetRoomUsers(s.ctx, "room1")
		s.Require().NotNil(users)
		s.Contains(users, "user1")
		s.Equal("anchor", users["user1"].Role)
		s.Equal(constants.AnchorStatusOnAir, users["user1"].Status)
	})
}

func (s *CombinedRoomTestSuite) TestCheckTimeout() {
	s.Run("check timeout with no users", func() {
		s.resetRoomState()

		roomIDs, err := s.room.CheckTimeout(s.ctx)
		s.Require().NoError(err)
		s.Len(roomIDs, 0)
	})

	s.Run("check timeout with expired user", func() {
		s.resetRoomState()

		oldTime := time.Now().Add(-users.UserStatusTimeout - time.Minute)

		_, _ = s.room.CreateUser(s.ctx, "room1", "user1", &users.User{
			Role: "anchor",
			Gen:  0,
			TS:   oldTime,
		})
		_, _ = s.room.UpdateUserStatus(s.ctx, "room1", "user1", &users.User{
			Status: constants.AnchorStatusIdle,
			Gen:    1,
			TS:     oldTime,
		})

		roomIDs, err := s.room.CheckTimeout(s.ctx)
		s.Require().NoError(err)
		s.Contains(roomIDs, "room1")

		users := s.room.GetRoomUsers(s.ctx, "room1")
		s.Equal(constants.AnchorStatus(""), users["user1"].Status)
	})

	s.Run("check timeout with active user", func() {
		s.resetRoomState()

		now := time.Now()

		_, _ = s.room.CreateUser(s.ctx, "room2", "user2", &users.User{
			Role: "anchor",
			Gen:  0,
			TS:   now,
		})
		_, _ = s.room.UpdateUserStatus(s.ctx, "room2", "user2", &users.User{
			Status: constants.AnchorStatusOnAir,
			Gen:    1,
			TS:     now,
		})

		roomIDs, err := s.room.CheckTimeout(s.ctx)
		s.Require().NoError(err)
		s.Len(roomIDs, 0)

		users := s.room.GetRoomUsers(s.ctx, "room2")
		s.Equal(constants.AnchorStatusOnAir, users["user2"].Status)
	})
}
