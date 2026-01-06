package room

import (
	"sync"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/zset"
	"github.com/imtaco/audio-rtc-exp/users"
)

type roomsStateMem struct {
	rooms map[string]map[string]*users.User // roomId -> userId -> User
	// TODO is rwLock needed ?! access is single threaded from controller
	rwLock sync.RWMutex

	userTracks *zset.Zset[string]
	roomTracks *zset.Zset[string]
	logger     *log.Logger
}

func (r *roomsStateMem) addRoomTrack(roomID string, ts time.Time) {
	r.roomTracks.Put(roomID, "", ts)
}

func (r *roomsStateMem) createRoomUser(roomID, userID string, u *users.User) bool {
	r.rwLock.Lock()
	defer r.rwLock.Unlock()

	var room map[string]*users.User
	var ok bool
	if room, ok = r.rooms[roomID]; !ok {
		room = make(map[string]*users.User)
		r.rooms[roomID] = room
	} else if _, ok = room[userID]; ok {
		return false
	}
	// newly created user only have role set
	room[userID] = &users.User{
		Role: u.Role,
		Gen:  u.Gen,
	}

	r.userTracks.Put(userID, roomID, u.TS)
	r.addRoomTrack(roomID, u.TS)

	return true
}

func (r *roomsStateMem) setUserStatus(roomID, userID string, u *users.User) bool {
	r.rwLock.Lock()
	defer r.rwLock.Unlock()

	var room map[string]*users.User
	var ok bool
	if room, ok = r.rooms[roomID]; !ok {
		return false
	}
	// no role
	ou, ok := room[userID]
	// TODO: check gen here ? need more thought and design
	if !ok || ou.Role == "" {
		return false
	}
	ou.Status = u.Status
	ou.Gen = u.Gen
	ou.TS = u.TS

	if u.Status == "" {
		// zero time for empty status
		u.TS = time.Time{}
		r.userTracks.Remove(userID)
	} else {
		r.userTracks.Put(userID, roomID, u.TS)
	}

	return true
}

func (r *roomsStateMem) removeRoomUser(roomID, userID string) (ok bool, lastUser bool) {
	r.rwLock.Lock()
	defer r.rwLock.Unlock()

	room, ok := r.rooms[roomID]
	if !ok {
		return false, false
	}
	if _, ok := room[userID]; !ok {
		return false, false
	}
	delete(room, userID)
	// remove from tracking
	r.userTracks.Remove(userID)

	if len(room) == 0 {
		delete(r.rooms, roomID)
		// remove room tracking
		r.roomTracks.Remove(roomID)
		return true, true
	}
	return true, false
}

func (r *roomsStateMem) getRoomUsers(roomID string) map[string]users.User {
	r.rwLock.RLock()
	defer r.rwLock.RUnlock()

	room, ok := r.rooms[roomID]
	if !ok {
		return nil
	}
	// Return a copy to avoid data races
	copied := make(map[string]users.User)
	for k, v := range room {
		copied[k] = *v
	}
	return copied
}

func ensureUser(us map[string]*users.User, userID string) *users.User {
	u, ok := us[userID]
	if ok {
		return u
	}
	u = &users.User{}
	us[userID] = u
	return u
}
