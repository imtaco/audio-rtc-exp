package service

import (
	"context"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	utils "github.com/imtaco/audio-rtc-exp/rooms/utils"
)

const (
	startTimeout           = 10 * time.Minute
	inactiveGracefulPeriod = 1 * time.Minute
	roomMaxAge             = 3 * time.Hour
)

func (rm *resourceMgrImpl) checkStaleRooms(ctx context.Context) error {
	// Get all rooms from etcd watcher cache
	rooms, err := rm.roomStore.GetAllRooms(ctx)
	if err != nil {
		return err
	}

	for roomID := range rooms {
		if err := rm.checkStaleRoom(ctx, roomID); err != nil {
			rm.logger.Error("Error during housekeeping a room",
				log.String("roomId", roomID),
				log.Error(err))
		}
	}

	return nil
}

func (rm *resourceMgrImpl) checkRoomModules(ctx context.Context) error {
	// Get all rooms from etcd watcher cache
	rooms, err := rm.roomStore.GetAllRooms(ctx)
	if err != nil {
		return err
	}

	for roomID := range rooms {
		if err := rm.checkRoomModule(ctx, roomID); err != nil {
			rm.logger.Error("Error during checking room module",
				log.String("roomId", roomID),
				log.Error(err))
		}
	}

	return nil
}

func (rm *resourceMgrImpl) checkStaleRoom(ctx context.Context, roomID string) error {
	staleRoomsChecked.Add(ctx, 1)

	state, ok := rm.roomWatcher.GetCachedState(roomID)
	if !ok {
		rm.logger.Debug("Room not found during housekeeping", log.String("roomId", roomID))
		return nil
	}

	meta := state.Meta
	livemeta := state.LiveMeta

	if meta == nil {
		rm.logger.Info("Deleting malformed room", log.String("roomId", roomID))
		malformedRoomsDeleted.Add(ctx, 1)
		staleRoomsDeleted.Add(ctx, 1)
		return rm.deleteRoom(ctx, roomID)
	}

	// check if room failed to start
	if livemeta == nil {
		if time.Since(meta.CreatedAt) > startTimeout {
			rm.logger.Info("Deleting inactive room", log.String("roomId", roomID))
			inactiveRoomsDeleted.Add(ctx, 1)
			staleRoomsDeleted.Add(ctx, 1)
			return rm.deleteRoom(ctx, roomID)
		}
	} else {
		// Check if room exceeded max age
		if livemeta.Status == constants.RoomStatusOnAir && time.Since(meta.CreatedAt) > roomMaxAge {
			rm.logger.Info("Deleting room exceeded max age", log.String("roomId", roomID))
			expiredRoomsDeleted.Add(ctx, 1)
			staleRoomsDeleted.Add(ctx, 1)
			return rm.deleteRoom(ctx, roomID)
		}

		// Check if room is in removing state and grace period has passed
		if livemeta.DiscardAt != nil && utils.IsExceed(*livemeta.DiscardAt, inactiveGracefulPeriod) {
			rm.logger.Info("Deleting inactive room after grace period", log.String("roomId", roomID))
			staleRoomsDeleted.Add(ctx, 1)
			return rm.deleteRoom(ctx, roomID)
		}
	}

	return nil
}

func (rm *resourceMgrImpl) checkRoomModule(ctx context.Context, roomID string) error {
	state, ok := rm.roomWatcher.GetCachedState(roomID)
	if !ok {
		return nil
	}

	livemeta := state.LiveMeta
	if livemeta == nil || livemeta.Status != constants.RoomStatusOnAir {
		return nil
	}

	// Check mixer health
	mixer, ok := rm.mixerWatcher.Get(livemeta.MixerID)
	if !ok || !mixer.IsStable() {
		unhealthyMixersDetected.Add(ctx, 1)
		rm.logger.Info("Mixer unhealthy or not ready, need to pick another",
			log.String("roomId", roomID),
			log.String("mixerId", livemeta.MixerID))
		// TODO: pick another mixer and update livemeta
	}

	// Check janus health
	janus, ok := rm.janusWatcher.Get(livemeta.JanusID)
	if !ok || !janus.IsStable() {
		unhealthyJanusesDetected.Add(ctx, 1)
		rm.logger.Info("Janus unhealthy or not ready, need to pick another",
			log.String("roomId", roomID),
			log.String("janusId", livemeta.JanusID))
		// TODO: pick another janus and update livemeta
		// how to notify andor for janus change ?
	}

	return nil
}

func (rm *resourceMgrImpl) deleteRoom(ctx context.Context, roomID string) error {
	// TODO: delete room in user service
	// last step
	_, err := rm.roomStore.DeleteRoom(ctx, roomID)
	return err
}
