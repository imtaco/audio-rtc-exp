package watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

func TestNewPortManager(t *testing.T) {
	t.Run("create port manager successfully", func(t *testing.T) {
		pm := NewPortManager(10000, 20000, log.NewNop())
		assert.NotNil(t, pm)

		impl := pm.(*portMgrImpl)
		assert.Equal(t, 10000, impl.portRangeStart)
		assert.Equal(t, 20000, impl.portRangeEnd)
	})
}

func TestGetFreeRTPPort(t *testing.T) {
	t.Run("allocate RTP port in range", func(t *testing.T) {
		pm := NewPortManager(50000, 50100, log.NewNop())

		port, err := pm.GetFreeRTPPort()

		assert.NoError(t, err)
		assert.Greater(t, port, 0)
		assert.True(t, port%2 == 0, "RTP port should be even")
		assert.LessOrEqual(t, port, 50100)
	})

	t.Run("port is even number", func(t *testing.T) {
		pm := NewPortManager(49152, 50000, log.NewNop())

		port, err := pm.GetFreeRTPPort()

		assert.NoError(t, err)
		assert.Equal(t, 0, port%2, "Port should be even (for RTP)")
	})

	t.Run("very small range", func(t *testing.T) {
		pm := NewPortManager(55000, 55010, log.NewNop())

		port, err := pm.GetFreeRTPPort()

		assert.NoError(t, err)
		assert.GreaterOrEqual(t, port, 55000)
	})
}

func TestTestUDPPort(t *testing.T) {
	pm := &portMgrImpl{
		portRangeStart: 10000,
		portRangeEnd:   20000,
		logger:         log.NewNop(),
	}

	t.Run("test valid port range", func(t *testing.T) {
		available := pm.testUDPPort(55555)
		assert.True(t, available, "High port should be available")
	})
}

func TestTestRTPRTCPPorts(t *testing.T) {
	pm := &portMgrImpl{
		portRangeStart: 10000,
		portRangeEnd:   20000,
		logger:         log.NewNop(),
	}

	t.Run("both ports available", func(t *testing.T) {
		available := pm.testRTPRTCPPorts(56000)
		assert.True(t, available)
	})

	t.Run("check RTP and RTCP pair", func(t *testing.T) {
		rtpPort := 56002
		available := pm.testRTPRTCPPorts(rtpPort)
		assert.True(t, available, "RTP port %d and RTCP port %d should be available", rtpPort, rtpPort+1)
	})
}
