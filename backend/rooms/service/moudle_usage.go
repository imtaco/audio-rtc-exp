package service

import (
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

func newModuleUsage(name string, logger *log.Logger) *moduleUsage {
	return &moduleUsage{
		name:    name,
		counts:  make(map[string]int),
		assigns: make(map[string]string),
		logger:  logger,
	}
}

type moduleUsage struct {
	name    string
	counts  map[string]int    // moudle id -> count
	assigns map[string]string // room id -> moudle id
	logger  *log.Logger
}

func (m *moduleUsage) set(roomID, newModuleID string) {
	oldModuleID := m.assigns[roomID]
	if oldModuleID == newModuleID {
		return
	}
	if oldModuleID != "" {
		m.counts[oldModuleID]--
		if m.counts[oldModuleID] <= 0 {
			delete(m.counts, oldModuleID)
		}
		delete(m.assigns, roomID)

		m.logger.Debug("Decremented module usage",
			log.String("moduleID", oldModuleID),
			log.Int("newCount", m.counts[oldModuleID]),
		)
	}
	if newModuleID != "" {
		m.counts[newModuleID]++
		m.assigns[roomID] = newModuleID

		m.logger.Debug("Incremented module usage",
			log.String("moduleID", newModuleID),
			log.Int("newCount", m.counts[newModuleID]),
		)
	}
}

func (m *moduleUsage) count(moduleID string) int {
	return m.counts[moduleID]
}
