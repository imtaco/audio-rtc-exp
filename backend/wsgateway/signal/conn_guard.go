package signal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/redis/go-redis/v9"
)

const (
	connLockTTL      = 30 * time.Second
	serverHBTTL      = 3 * time.Second
	serverHBInterval = time.Second
	redisTimeout     = 2 * time.Second
)

var (
	// Lua script for acquiring connection lock
	// KEYS[1]: lock key (user lock)
	// KEYS[2]: server heartbeat key
	// ARGV[1]: lock value (serverID:nonce)
	// ARGV[2]: lock TTL in milliseconds
	luaAcquireConnLock = redis.NewScript(`
		local cur = redis.call('GET', KEYS[1])
		if cur == false then
			redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
			return 1
		end

		if cur == ARGV[1] then
			redis.call('PEXPIRE', KEYS[1], ARGV[2])
			return 1
		end

		local svExists = redis.call('EXISTS', KEYS[2])
		if svExists == 0 then
			redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
			return 1
		end

		return 0
	`)

	// Lua script for releasing connection lock
	// KEYS[1]: lock key
	// ARGV[1]: lock value (serverID:nonce)
	luaReleaseConnLock = redis.NewScript(`
		local cur = redis.call('GET', KEYS[1])
		if cur ~= ARGV[1] then
			return 0
		end
		redis.call('DEL', KEYS[1])
		return 1
	`)
)

type connGuardImpl struct {
	redisClient *redis.Client
	prefix      string
	serverID    string
	logger      *log.Logger

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewConnGuard(
	redisClient *redis.Client,
	redisPrefix string,
	serverID string,
	logger *log.Logger,
) ConnectionGuard {
	return &connGuardImpl{
		redisClient: redisClient,
		prefix:      redisPrefix,
		serverID:    serverID,
		logger:      logger,
		stopCh:      make(chan struct{}),
	}
}

func (s *connGuardImpl) connKey(userID string) string {
	return fmt.Sprintf("%s:c:%s", s.prefix, userID)
}

func (s *connGuardImpl) serverKey() string {
	return fmt.Sprintf("%s:s:%s", s.prefix, s.serverID)
}

func (s *connGuardImpl) lockValue(nonce string) string {
	return fmt.Sprintf("%s:%s", s.serverID, nonce)
}

func (s *connGuardImpl) GetServerID() string {
	return s.serverID
}

func (s *connGuardImpl) MustHold(mctx jsonrpc.MethodContext[rtcContext]) (bool, error) {
	rtcCtx := mctx.Get()

	s.logger.Debug("Acquiring connect lock",
		log.String("userId", rtcCtx.userID),
		log.String("nonce", rtcCtx.connID),
		log.String("serverId", s.serverID),
	)

	lockVal := s.lockValue(rtcCtx.connID)
	serverKey := s.serverKey()

	result, err := luaAcquireConnLock.Run(
		rtcCtx.reqCtx,
		s.redisClient,
		[]string{s.connKey(rtcCtx.userID), serverKey},
		lockVal,
		connLockTTL.Microseconds(),
	).Int()

	if err != nil {
		return false, fmt.Errorf("fail to acquire lock: %w", err)
	}
	if result == 1 {
		return true, nil
	}

	// TODO; close connection gracefully, and send proper error code/message to avoid reconnection
	mctx.Peer().Close()
	s.logger.Debug("Connection rejected due to existing connection",
		log.String("connId", rtcCtx.connID),
		log.String("userId", rtcCtx.userID),
	)
	return false, nil
}

func (s *connGuardImpl) Release(mctx jsonrpc.MethodContext[rtcContext]) error {
	rtcCtx := mctx.Get()

	s.logger.Debug("Releasing connect lock",
		log.String("userId", rtcCtx.userID),
		log.String("nonce", rtcCtx.connID),
		log.String("serverId", s.serverID),
	)

	lockVal := s.lockValue(rtcCtx.connID)
	_, err := luaReleaseConnLock.Run(
		rtcCtx.reqCtx,
		s.redisClient,
		[]string{s.connKey(rtcCtx.userID)},
		lockVal,
	).Int()

	if err != nil {
		return fmt.Errorf("fail to release lock: %w", err)
	}
	return nil
}

func (s *connGuardImpl) Start(ctx context.Context) error {
	s.logger.Info("Starting server heartbeat", log.String("serverId", s.serverID))

	if err := s.setHearbeat(ctx); err != nil {
		return fmt.Errorf("failed to set initial heartbeat: %w", err)
	}

	s.wg.Add(1)
	go s.heartbeatLoop()

	return nil
}

func (s *connGuardImpl) Stop() {
	s.logger.Info("Stopping server heartbeat", log.String("serverId", s.serverID))
	close(s.stopCh)
	s.wg.Wait()
}

func (s *connGuardImpl) setHearbeat(ctx context.Context) error {
	return s.redisClient.Set(
		ctx, s.serverKey(),
		"1",
		serverHBTTL).Err()
}

func (s *connGuardImpl) heartbeatLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(serverHBInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
			defer cancel()
			s.redisClient.Del(ctx, s.serverKey())
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
			if err := s.setHearbeat(ctx); err != nil {
				s.logger.Error("Failed to extend server heartbeat", log.Error(err))
			}
			cancel()
		}
	}
}
