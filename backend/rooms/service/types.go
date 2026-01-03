package service

import (
	reswatcher "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd"
)

// RoomWatcherWithStats extends RoomWatcher with module usage statistics
type RoomWatcherWithStats interface {
	reswatcher.RoomWatcher
	GetJanusStreamCount(janusID string) int
	GetMixerStreamCount(mixerID string) int
}
