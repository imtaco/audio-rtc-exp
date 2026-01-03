package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/utils"
	"github.com/imtaco/audio-rtc-exp/rooms"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type roomStoreImpl struct {
	etcdClient etcd.Client
	prefix     string
	logger     *log.Logger
}

func NewRoomStore(etcdClient etcd.Client, prefix string, logger *log.Logger) rooms.RoomStore {
	return &roomStoreImpl{
		etcdClient: etcdClient,
		prefix:     prefix,
		logger:     logger,
	}
}

func (rs *roomStoreImpl) metaKey(roomID string) string {
	return fmt.Sprintf("%s%s/%s", rs.prefix, roomID, constants.RoomKeyMeta)
}

func (rs *roomStoreImpl) livemetaKey(roomID string) string {
	return fmt.Sprintf("%s%s/%s", rs.prefix, roomID, constants.RoomKeyLiveMeta)
}

func (rs *roomStoreImpl) mixerKey(roomID string) string {
	return fmt.Sprintf("%s%s/%s", rs.prefix, roomID, constants.RoomKeyMixer)
}

func (rs *roomStoreImpl) CreateRoom(ctx context.Context, roomID string, roomData *etcdstate.Meta) (*etcdstate.Meta, error) {
	metaKey := rs.metaKey(roomID)
	rs.logger.Info("create room with key", log.String("metaKey", metaKey))

	// Check if room already exists
	resp, err := rs.etcdClient.Get(ctx, metaKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check room existence: %w", err)
	}
	if len(resp.Kvs) > 0 {
		return nil, fmt.Errorf("room %s already exists", roomID)
	}

	// Set creation timestamp
	roomData.CreatedAt = time.Now().UTC()

	// Marshal to JSON
	data, err := json.Marshal(roomData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room data: %w", err)
	}

	// Store in etcd
	_, err = rs.etcdClient.Put(ctx, metaKey, string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to store room: %w", err)
	}

	rs.logger.Info("Created room", log.String("roomId", roomID))
	return roomData, nil
}

func (rs *roomStoreImpl) GetRoom(ctx context.Context, roomID string) (*etcdstate.Meta, error) {
	metaKey := rs.metaKey(roomID)
	resp, err := rs.etcdClient.Get(ctx, metaKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, nil
	}

	var room etcdstate.Meta
	if err := json.Unmarshal(resp.Kvs[0].Value, &room); err != nil {
		return nil, fmt.Errorf("failed to unmarshal room data: %w", err)
	}

	return &room, nil
}

func (rs *roomStoreImpl) Exists(ctx context.Context, roomID string) (bool, error) {
	metaKey := rs.metaKey(roomID)
	rs.logger.Info("Check room existence", log.String("metaKey", metaKey))

	resp, err := rs.etcdClient.Get(ctx, metaKey)
	if err != nil {
		return false, fmt.Errorf("failed to check room existence: %w", err)
	}

	exists := len(resp.Kvs) > 0
	rs.logger.Info("Value", log.Bool("exists", exists))
	return exists, nil
}

func (rs *roomStoreImpl) StopRoom(ctx context.Context, roomID string) error {
	return rs.StopLiveMeta(ctx, roomID)
}

func (rs *roomStoreImpl) DeleteRoom(ctx context.Context, roomID string) (bool, error) {
	roomPrefix := fmt.Sprintf("%s%s/", rs.prefix, roomID)

	// Delete all keys with prefix /rooms/<room_id>/
	resp, err := rs.etcdClient.Delete(ctx, roomPrefix, clientv3.WithPrefix())
	if err != nil {
		return false, fmt.Errorf("failed to delete room: %w", err)
	}

	if resp.Deleted == 0 {
		rs.logger.Info("Room not found", log.String("roomId", roomID))
		return false, nil
	}

	rs.logger.Info("Deleted room", log.String("roomId", roomID), log.Int64("deleted", resp.Deleted))
	return true, nil
}

func (rs *roomStoreImpl) CreateLiveMeta(ctx context.Context, roomID, mixerID, janusID, nonce string) error {
	livemetaKey := rs.livemetaKey(roomID)
	rs.logger.Info("Starting livemeta for room", log.String("roomId", roomID))

	livemeta := etcdstate.LiveMeta{
		Status:    constants.RoomStatusOnAir,
		MixerID:   mixerID,
		JanusID:   janusID,
		Nonce:     nonce,
		CreatedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(livemeta)
	if err != nil {
		return fmt.Errorf("failed to marshal livemeta: %w", err)
	}

	_, err = rs.etcdClient.Put(ctx, livemetaKey, string(data))
	if err != nil {
		return fmt.Errorf("failed to store livemeta: %w", err)
	}

	rs.logger.Info("Written livemeta for room",
		log.String("roomId", roomID),
		log.Any("livemeta", livemeta))
	return nil
}

func (rs *roomStoreImpl) StopLiveMeta(ctx context.Context, roomID string) error {
	livemetaKey := rs.livemetaKey(roomID)
	rs.logger.Info("Stopping livemeta for room", log.String("roomId", roomID))

	livemeta := etcdstate.LiveMeta{
		Status:    constants.RoomStatusRemoving,
		DiscardAt: utils.Ptr(time.Now().UTC()),
	}

	data, err := json.Marshal(livemeta)
	if err != nil {
		return fmt.Errorf("failed to marshal livemeta: %w", err)
	}

	_, err = rs.etcdClient.Put(ctx, livemetaKey, string(data))
	if err != nil {
		return fmt.Errorf("failed to store livemeta: %w", err)
	}

	rs.logger.Info("Written livemeta for room",
		log.String("roomId", roomID),
		log.Any("livemeta", livemeta))
	return nil
}

func (rs *roomStoreImpl) GetAllRooms(ctx context.Context) (map[string]*etcdstate.Meta, error) {
	resp, err := rs.etcdClient.Get(ctx, rs.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get all rooms: %w", err)
	}

	rms := make(map[string]*etcdstate.Meta)
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Filter only meta keys
		if len(key) >= 5 && key[len(key)-5:] == "/meta" {
			var room etcdstate.Meta
			if err := json.Unmarshal(kv.Value, &room); err != nil {
				rs.logger.Error("Failed to unmarshal room data",
					log.String("key", key),
					log.Error(err))
				continue
			}
			// Extract roomID from key: prefix + roomID + /meta
			roomID := key[len(rs.prefix) : len(key)-5]
			rms[roomID] = &room
		}
	}

	return rms, nil
}

func (rs *roomStoreImpl) GetStats(ctx context.Context) (*rooms.RoomStats, error) {
	rms, err := rs.GetAllRooms(ctx)
	if err != nil {
		return nil, err
	}

	stats := &rooms.RoomStats{
		Total:             len(rms),
		TotalParticipants: 0, // Not tracking participants in this version
	}

	return stats, nil
}

func (rs *roomStoreImpl) GetMixerData(ctx context.Context, roomID string) (*etcdstate.Mixer, error) {
	mixerKey := rs.mixerKey(roomID)
	resp, err := rs.etcdClient.Get(ctx, mixerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get mixer data: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, nil
	}

	var mixerData etcdstate.Mixer
	if err := json.Unmarshal(resp.Kvs[0].Value, &mixerData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mixer data: %w", err)
	}

	return &mixerData, nil
}

func (rs *roomStoreImpl) moduleMarkKey(moduleType, moduleID string) string {
	return fmt.Sprintf("%s%s/%s", moduleType, moduleID, constants.ModuleKeyMark)
}

func (rs *roomStoreImpl) SetModuleMark(ctx context.Context, moduleType, moduleID string, label constants.MarkLabel, ttlSeconds int64) error {
	markKey := rs.moduleMarkKey(moduleType, moduleID)
	rs.logger.Info("Setting module mark",
		log.String("moduleType", moduleType),
		log.String("moduleID", moduleID),
		log.String("label", string(label)),
		log.Int64("ttl", ttlSeconds))

	markData := etcdstate.MarkData{
		Label: label,
	}

	data, err := json.Marshal(markData)
	if err != nil {
		return fmt.Errorf("failed to marshal mark data: %w", err)
	}

	// Create lease if TTL is specified
	var opts []clientv3.OpOption
	if ttlSeconds > 0 {
		lease, err := rs.etcdClient.Grant(ctx, ttlSeconds)
		if err != nil {
			return fmt.Errorf("failed to create lease: %w", err)
		}
		opts = append(opts, clientv3.WithLease(lease.ID))
		rs.logger.Info("Created lease for module mark",
			log.String("moduleID", moduleID),
			log.Int64("leaseID", int64(lease.ID)),
			log.Int64("ttl", ttlSeconds))
	}

	_, err = rs.etcdClient.Put(ctx, markKey, string(data), opts...)
	if err != nil {
		return fmt.Errorf("failed to set module mark: %w", err)
	}

	rs.logger.Info("Set module mark successfully",
		log.String("moduleType", moduleType),
		log.String("moduleID", moduleID),
		log.String("label", string(label)))
	return nil
}

func (rs *roomStoreImpl) DeleteModuleMark(ctx context.Context, moduleType, moduleID string) error {
	markKey := rs.moduleMarkKey(moduleType, moduleID)
	rs.logger.Info("Deleting module mark",
		log.String("moduleType", moduleType),
		log.String("moduleID", moduleID))

	_, err := rs.etcdClient.Delete(ctx, markKey)
	if err != nil {
		return fmt.Errorf("failed to delete module mark: %w", err)
	}

	rs.logger.Info("Deleted module mark successfully",
		log.String("moduleType", moduleType),
		log.String("moduleID", moduleID))
	return nil
}
