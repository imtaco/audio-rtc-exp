package etcdstate

import (
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
)

// RoomState represents the complete state of a room
type RoomState struct {
	Meta     *Meta
	LiveMeta *LiveMeta
	Mixer    *Mixer
	Janus    *Janus
}

// IsEmpty checks if the room state is empty
func (rs *RoomState) IsEmpty() bool {
	return rs == nil || (rs.Meta == nil && rs.LiveMeta == nil && rs.Mixer == nil && rs.Janus == nil)
}

// GetMeta gets the meta for the room
func (rs *RoomState) GetMeta() *Meta {
	if rs == nil {
		return nil
	}
	return rs.Meta
}

// GetLiveMeta gets the livemeta for the room
func (rs *RoomState) GetLiveMeta() *LiveMeta {
	if rs == nil {
		return nil
	}
	return rs.LiveMeta
}

// GetMixer gets the mixer data for the room
func (rs *RoomState) GetMixer() *Mixer {
	if rs == nil {
		return nil
	}
	return rs.Mixer
}

// GetJanus gets the Janus data for the room
func (rs *RoomState) GetJanus() *Janus {
	if rs == nil {
		return nil
	}
	return rs.Janus
}

// SetMeta sets the meta for the room
func (rs *RoomState) SetMeta(m *Meta) {
	if rs == nil {
		return
	}
	rs.Meta = m
}

// SetLiveMeta sets the livemeta for the room
func (rs *RoomState) SetLiveMeta(lm *LiveMeta) {
	if rs == nil {
		return
	}
	rs.LiveMeta = lm
}

// SetMixer sets the mixer data for the room
func (rs *RoomState) SetMixer(m *Mixer) {
	if rs == nil {
		return
	}
	rs.Mixer = m
}

// SetJanus sets the Janus data for the room
func (rs *RoomState) SetJanus(j *Janus) {
	if rs == nil {
		return
	}
	rs.Janus = j
}

// LiveMeta represents the livemeta data from etcd
type LiveMeta struct {
	Status    constants.RoomStatus `json:"status"`
	MixerID   string               `json:"mixerId"`
	JanusID   string               `json:"janusId"`
	CreatedAt time.Time            `json:"createdAt"`
	DiscardAt *time.Time           `json:"discardAt,omitempty"`
	Nonce     string               `json:"nonce"`
}

func (m *LiveMeta) GetStatus() constants.RoomStatus {
	if m == nil {
		return ""
	}
	return m.Status
}

func (m *LiveMeta) GetMixerID() string {
	if m == nil {
		return ""
	}
	return m.MixerID
}

func (m *LiveMeta) GetJanusID() string {
	if m == nil {
		return ""
	}
	return m.JanusID
}
func (m *LiveMeta) GetNonce() string {
	if m == nil {
		return ""
	}
	return m.Nonce
}
func (m *LiveMeta) GetCreatedAt() time.Time {
	if m == nil {
		return time.Time{}
	}
	return m.CreatedAt
}
func (m *LiveMeta) GetDiscardAt() *time.Time {
	if m == nil {
		return nil
	}
	return m.DiscardAt
}

// MetaData contains metadata about a room
type Meta struct {
	Pin        string    `json:"pin"`
	HLSPath    string    `json:"hlsPath"`
	MaxAnchors int       `json:"maxAnchors"`
	CreatedAt  time.Time `json:"createdAt,omitempty"`
}

func (m *Meta) GetPin() string {
	if m == nil {
		return ""
	}
	return m.Pin
}

func (m *Meta) GetHLSPath() string {
	if m == nil {
		return ""
	}
	return m.HLSPath
}

func (m *Meta) GetMaxAnchors() int {
	if m == nil {
		return 0
	}
	return m.MaxAnchors
}

func (m *Meta) GetCreatedAt() time.Time {
	if m == nil {
		return time.Time{}
	}
	return m.CreatedAt
}
