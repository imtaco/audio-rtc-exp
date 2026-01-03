package etcdstate

import (
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
)

// HeartbeatData represents the heartbeat data structure
type HeartbeatData struct {
	Status    string    `json:"status"`
	Host      string    `json:"host"`
	Capacity  int       `json:"capacity"`
	StartedAt time.Time `json:"startedAt"` // StartedAt is the timestamp when the module started
}

func (h *HeartbeatData) GetStatus() string {
	if h != nil {
		return h.Status
	}
	return ""
}

func (h *HeartbeatData) GetHost() string {
	if h != nil {
		return h.Host
	}
	return ""
}

func (h *HeartbeatData) GetCapacity() int {
	if h != nil {
		return h.Capacity
	}
	return 0
}

func (h *HeartbeatData) GetStartedAt() time.Time {
	if h != nil {
		return h.StartedAt
	}
	return time.Time{}
}

// MarkData represents the mark data structure
type MarkData struct {
	Label constants.MarkLabel `json:"label"`
}

func (m *MarkData) GetLabel() constants.MarkLabel {
	if m != nil {
		return m.Label
	}
	return ""
}

// ModuleState represents the complete state data for a module
type ModuleState struct {
	Heartbeat *HeartbeatData `json:"heartbeat,omitempty"`
	Mark      *MarkData      `json:"mark,omitempty"`
}

// Getter methods with nil-safe access (protobuf-style)

func (m *ModuleState) IsEmpty() bool {
	return m == nil || (m.Heartbeat == nil && m.Mark == nil)
}

func (m *ModuleState) GetHeartbeat() *HeartbeatData {
	if m != nil {
		return m.Heartbeat
	}
	return nil
}

func (m *ModuleState) SetHeartbeat(hb *HeartbeatData) {
	if m != nil {
		m.Heartbeat = hb
	}
}

func (m *ModuleState) GetMark() *MarkData {
	if m != nil {
		return m.Mark
	}
	return nil
}

func (m *ModuleState) SetMark(mk *MarkData) {
	if m != nil {
		m.Mark = mk
	}
}

func (m *ModuleState) IsHealthy() bool {
	return m.GetHeartbeat().GetStatus() == constants.ModuleStatusHealthy
}

// IsPickableModule checks if a module is healthy and ready (can be picked for new rooms)
func (m *ModuleState) IsPickable() bool {
	if m == nil {
		return false
	}
	// No label means ready (default)
	label := m.GetMark().GetLabel()
	if label == "" {
		label = constants.MarkLabelReady
	}
	// Only ready and healthy is pickable
	return m.GetHeartbeat().GetStatus() == constants.ModuleStatusHealthy &&
		label == constants.MarkLabelReady
}

func (m *ModuleState) IsStable() bool {
	if m == nil {
		return false
	}
	// No label means ready (default)
	label := m.GetMark().GetLabel()
	if label == "" {
		label = constants.MarkLabelReady
	}
	// Healthy and either ready or cordoned
	return m.GetHeartbeat().GetStatus() == constants.ModuleStatusHealthy &&
		(label == constants.MarkLabelReady || label == constants.MarkLabelCordon)
}
