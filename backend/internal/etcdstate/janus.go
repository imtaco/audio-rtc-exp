package etcdstate

import "time"

// Janus is written to etcd to track Janus room status
type Janus struct {
	JanusID     string    `json:"janusId"`
	Status      string    `json:"status"`
	Timestamp   time.Time `json:"timestamp"`
	JanusRoomID int64     `json:"janusRoomId,omitempty"`
}

func (j *Janus) GetJanusID() string {
	if j == nil {
		return ""
	}
	return j.JanusID
}

func (j *Janus) GetStatus() string {
	if j == nil {
		return ""
	}
	return j.Status
}

func (j *Janus) GetTimestamp() time.Time {
	if j == nil {
		return time.Time{}
	}
	return j.Timestamp
}

func (j *Janus) GetJanusRoomID() int64 {
	if j == nil {
		return 0
	}
	return j.JanusRoomID
}
