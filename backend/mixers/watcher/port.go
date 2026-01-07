package watcher

import (
	"fmt"
	"math/rand/v2"
	"net"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/mixers"
)

// portMgrImpl handles RTP/RTCP port allocation
type portMgrImpl struct {
	portRangeStart int
	portRangeEnd   int
	logger         *log.Logger
}

// NewPortManager creates a new portMgrImpl
func NewPortManager(portRangeStart, portRangeEnd int, logger *log.Logger) mixers.PortManager {
	return &portMgrImpl{
		portRangeStart: portRangeStart,
		portRangeEnd:   portRangeEnd,
		logger:         logger,
	}
}

// GetFreeRTPPort finds a free RTP/RTCP port pair within the specified range
// Returns the RTP port (even number), RTCP will be RTP + 1
func (pm *portMgrImpl) GetFreeRTPPort() (int, error) {
	maxAttempts := 10

	// Try to find a port pair in the specified range
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Generate random even port in range (RTP must be even)
		port := pm.portRangeStart + rand.IntN(pm.portRangeEnd-pm.portRangeStart+1) // #nosec G404 -- weak random is acceptable for port selection, no security impact

		// Ensure port is even
		if port%2 != 0 {
			port--
		}

		// Make sure we have room for RTCP (+1)
		if port >= pm.portRangeEnd {
			continue
		}

		// Test if both RTP and RTCP ports are available
		if pm.testRTPRTCPPorts(port) {
			return port, nil
		}
	}

	// Fallback: try ephemeral port range
	pm.logger.Warn("Could not find free RTP/RTCP port pair in configured range, trying ephemeral range",
		log.Int("start", pm.portRangeStart),
		log.Int("end", pm.portRangeEnd))

	// Try to find even port in ephemeral range
	ephemeralStart := 49152
	ephemeralEnd := 65534 // -1 to leave room for RTCP

	for i := 0; i < 20; i++ {
		port := ephemeralStart + rand.IntN(ephemeralEnd-ephemeralStart+1) // #nosec G404 -- weak random is acceptable for port selection, no security impact

		// Ensure port is even
		if port%2 != 0 {
			port--
		}

		if pm.testRTPRTCPPorts(port) {
			return port, nil
		}
	}

	return 0, fmt.Errorf("could not find available RTP/RTCP port pair")
}

// testUDPPort tests if a specific UDP port is available
func (pm *portMgrImpl) testUDPPort(port int) bool {
	addr := &net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: port,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

// testRTPRTCPPorts tests if both RTP (even) and RTCP (odd, +1) ports are available
func (pm *portMgrImpl) testRTPRTCPPorts(rtpPort int) bool {
	rtcpPort := rtpPort + 1

	if !pm.testUDPPort(rtpPort) {
		return false
	}

	return pm.testUDPPort(rtcpPort)
}
