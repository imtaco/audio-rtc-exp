package ffmpeg

import (
	"bufio"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

const (
	forceKillTimeout = 5 * time.Second
	retryDelay       = 2 * time.Second
)

func NewProcessInfo(
	roomID string,
	rtpPort int,
	sdpPath, hlsDir, keyInfoPath string,
	initSeq int,
	logger *log.Logger,
) *ProcessInfo {
	return &ProcessInfo{
		roomID:      roomID,
		rtpPort:     rtpPort,
		sdpPath:     sdpPath,
		hlsDir:      hlsDir,
		keyInfoPath: keyInfoPath,
		initSeq:     initSeq,
		chanStop:    make(chan struct{}),
		curSeq:      atomic.Pointer[int]{},
		SpawnFFmpeg: spawnFFmpeg, // Default implementation
		logger:      logger,
	}
}

// ProcessInfo tracks information about a running FFmpeg process
type ProcessInfo struct {
	// Immutable fields (no lock needed)
	roomID      string
	rtpPort     int
	sdpPath     string
	hlsDir      string
	keyInfoPath string
	initSeq     int

	pid      int32
	process  *exec.Cmd
	chanStop chan struct{}

	// Atomic fields for lock-free concurrent access
	curSeq atomic.Pointer[int]

	// Function for spawning FFmpeg process (can be replaced for testing)
	SpawnFFmpeg func(sdpPath, hlsDir string, startNumber int, keyInfoPath string) *exec.Cmd

	logger *log.Logger
}

func (p *ProcessInfo) Start() {
	go p.Run()
}

func (p *ProcessInfo) Run() {
	attempts := 0
	for {
		// select for ctx
		select {
		case <-p.chanStop:
			p.logger.Info("FFmpeg process stopping",
				log.String("roomId", p.roomID))
			return
		default:
		}

		if attempts > 0 {
			// exponential backoff with max cap
			time.Sleep(retryDelay)
		}

		p.logger.Info("FFmpeg retry attempt",
			log.String("roomId", p.roomID),
			log.Int("attempt", attempts))

		p.runOnce()
		attempts++
	}
}

func (p *ProcessInfo) Stop() {
	// might close channel multiple times, recover from panic
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("panic during FFmpeg stop",
				log.Any("error", r))
		}
	}()

	close(p.chanStop)
}

// Start starts the FFmpeg process
func (p *ProcessInfo) runOnce() {
	// Determine start number
	startNumber := p.initSeq
	curSeqPtr := p.curSeq.Load()
	if curSeqPtr != nil {
		startNumber = *curSeqPtr + 1
	}

	// Read attempt atomically

	p.logger.Info("FFmpeg attempt",
		log.String("roomId", p.roomID),
		log.Int("startNumber", startNumber))

	cmd := p.SpawnFFmpeg(p.sdpPath, p.hlsDir, startNumber, p.keyInfoPath)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		p.logger.Error("Failed to start FFmpeg", log.String("roomId", p.roomID), log.Error(err))
		return
	}

	// Store PID atomically
	// #nosec G115 -- Process.Pid is guaranteed to fit in int32 on all platforms
	p.pid = int32(cmd.Process.Pid)
	p.process = cmd

	// Handle stdout
	go p.handleStdout(stdout)

	// Handle stderr
	go p.handleStderr(stderr)

	// Wait for process to exit
	done := p.startWaitForExit()

	select {
	case <-done:
	case <-p.chanStop:
		p.stop()
		// still need to wait for done
		<-done
	}
}

// Stop stops the FFmpeg process
func (p *ProcessInfo) stop() {
	logger := p.logger
	logger.Info("Stopping FFmpeg process",
		log.String("roomId", p.roomID),
		log.Int32("pid", p.pid))

	// Kill FFmpeg process if running
	if p.process == nil || p.process.Process == nil {
		return
	}

	if err := p.process.Process.Signal(syscall.SIGTERM); err != nil {
		logger.Error("Failed to send SIGTERM to FFmpeg process",
			log.String("roomId", p.roomID),
			log.Int32("pid", p.pid),
			log.Error(err))
	}
	// Force kill after timeout
	go func(cmd *exec.Cmd, pid int32, roomID string) {
		time.Sleep(forceKillTimeout)
		if cmd.Process != nil {
			p.logger.Info("Force killing FFmpeg", log.String("roomId", roomID), log.Int32("pid", pid))
			if err := cmd.Process.Kill(); err != nil {
				p.logger.Error("Failed to force kill FFmpeg process",
					log.String("roomId", roomID),
					log.Int32("pid", pid),
					log.Error(err))
			}
		}
	}(p.process, p.pid, p.roomID)
}

// handleStdout reads and logs FFmpeg stdout
func (p *ProcessInfo) handleStdout(stdout io.ReadCloser) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		p.logger.Debug("FFmpeg stdout", log.String("roomId", p.roomID), log.String("output", line))
	}
}

// handleStderr reads and logs FFmpeg stderr, extracting sequence numbers
func (p *ProcessInfo) handleStderr(stderr io.ReadCloser) {
	scanner := bufio.NewScanner(stderr)
	segmentRegex := regexp.MustCompile(`Opening '.*\/segment_(\d+)\.ts' for writing`)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		matches := segmentRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		// Parse segment filename to extract sequence number
		sequence, _ := strconv.Atoi(matches[1])
		if sequence <= 0 {
			continue
		}

		completedSeq := sequence - 1
		p.curSeq.Store(&completedSeq)

		p.logger.Debug("HLS Segment completed",
			log.String("roomId", p.roomID),
			log.Int("curSeq", completedSeq),
			log.Int("nextSeq", sequence))
	}
}

func (p *ProcessInfo) startWaitForExit() <-chan struct{} {
	done := make(chan struct{})
	cmd := p.process

	go func() {
		err := cmd.Wait()
		close(done)

		var exitCode int
		if err != nil {
			if exitErr, ok := errors.As[*exec.ExitError](err); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
				}
			}
		}

		// Check if SIGTERM was sent
		if exitCode == 143 || cmd.ProcessState.String() == "signal: terminated" {
			p.logger.Info("FFmpeg stopped (SIGTERM), not retrying", log.String("roomId", p.roomID))
			return
		}

		// Any other exit triggers retry
		p.logger.Info("FFmpeg stopped unexpectedly",
			log.String("roomId", p.roomID),
			log.Int("exitCode", exitCode))
	}()

	return done
}

// spawnFFmpeg spawns a new FFmpeg process
func spawnFFmpeg(sdpPath, hlsDir string, startNumber int, keyInfoPath string) *exec.Cmd {
	args := []string{
		"-protocol_whitelist", "file,udp,rtp",
		"-i", sdpPath,
		"-c:a", "aac",
		"-b:a", "48k",
		"-ar", "44100",
		"-ac", "1",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "5",
		"-hls_flags", "delete_segments",
		"-hls_start_number_source", "generic",
		"-start_number", strconv.Itoa(startNumber),
	}

	// Add encryption parameters if keyInfoPath is provided
	if keyInfoPath != "" {
		args = append(args, "-hls_key_info_file", keyInfoPath)
	}

	args = append(args,
		"-hls_segment_filename", filepath.Join(hlsDir, "segment_%03d.ts"),
		filepath.Join(hlsDir, "stream.m3u8"),
	)

	cmd := exec.Command("ffmpeg", args...)
	return cmd
}
