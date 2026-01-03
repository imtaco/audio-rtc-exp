package etcdstate

// Mixer represents the mixer data in etcd
type Mixer struct {
	ID   string `json:"id"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

func (m *Mixer) GetID() string {
	if m == nil {
		return ""
	}
	return m.ID
}

func (m *Mixer) GetIP() string {
	if m == nil {
		return ""
	}
	return m.IP
}

func (m *Mixer) GetPort() int {
	if m == nil {
		return 0
	}
	return m.Port
}
