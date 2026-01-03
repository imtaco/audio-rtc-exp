import { writable, get } from 'svelte/store';
import { WSRpcClient } from '../shared/ws-rpc.js';
import { ANCHOR_STATUS } from '../shared/constants.js';

// Constants
const KEEPALIVE_INTERVAL_MS = 15000;
const MAX_LOGS_SIZE = 100;

// Svelte stores for reactive state
export const clientId = writable(window.crypto.randomUUID().toString());
export const logs = writable([]);
export const statusMessage = writable('');
export const statusType = writable('info');
export const currentStep = writable('unconnected');
export const members = writable([]);
export const showAudioCtrl = writable(false);
export const connStatus = writable('disconnected');

export class AnchorClient {

  constructor(config = {}) {
    clientId.set(config.clientId || window.crypto.randomUUID().toString());

    this.wsUrl = config.wsUrl || 'ws://localhost:8081';
    this.token = config.token || '';
    this.pin = config.pin || '';
    this.displayName = config.displayName || 'Browser User';
    this.wavUrl = config.wavUrl || 'http://localhost:8080/sounds/speech_mono.wav';
    this.currentStep = 'unconnected';

    // State
    this.peer = null;
    this.pc = null;
    this.localStream = null;
    this.remoteStream = null;

    // UI elements (to be bound from Svelte)
    this.remoteAudioEl = null;
    this.playerEl = null;
  }

  register() {
    // sig.registerExceptionHandler((type, msg) => {
    //   console.error('ws exception', type, msg);
    // });

    this.peer.def('roomStatus', (pmembers) => {
      // this.log(`Peer joined: ${userId}`);
      console.log(`Room status update: ${JSON.stringify(pmembers)}`);
      members.set(pmembers);
    });
  }

  log(message) {
    console.log(message);
    logs.update(current => {
      const newLogs = [...current, { time: new Date().toLocaleTimeString(), message }];
      // Remove old logs if exceeds max size
      if (newLogs.length > MAX_LOGS_SIZE) {
        return newLogs.slice(newLogs.length - MAX_LOGS_SIZE);
      }
      return newLogs;
    });
  }

  setStatus(message, type = 'info') {
    statusMessage.set(message);
    statusType.set(type);
  }

  setStep(step) {
    this.currentStep = step;
    currentStep.set(step);
  }

  isPlaying() {
    const player = this.playerEl;
    return player && !player.paused && !player.ended && player.currentTime > 0;
  }

  async setupAudioStream() {
    const log = this.log.bind(this);

    if (this.wavUrl) {
      this.playerEl.src = this.wavUrl;
      this.playerEl.loop = true;
      await this.playerEl.play();
      log('Audio started (looping enabled)');
      this.localStream = this.playerEl.captureStream();
      log(`Captured stream tracks: ${this.localStream.getAudioTracks().length}`);
    } else {
      log('Requesting microphone access...');
      this.localStream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
      log('Microphone access granted');
    }
  }

  async connect() {
    const log = this.log.bind(this);
    const setStatus = this.setStatus.bind(this);

    log(`Connecting to Room Manager at ${this.wsUrl}...`);
    setStatus('Connecting...', 'info');

    this.peer = new WSRpcClient(this.wsUrl, this.token);

    try {
      await this.peer.connect();
    } catch (err) {
      log(`Failed to connect to WebSocket: ${err}`, 'error');
      throw err;
    }

    connStatus.set('connected');

    this.register();

    log('Connected to WebSocket');
    setStatus('Connected', 'success');

    this.peer.on('error', (error) => {
      log(`Socket error: ${error}`);
      setStatus('Connection error', 'error');
    });

    this.peer.on('close_normal', () => {
      log('Socket disconnected (normal)');
      setStatus('Disconnected', 'info');
      connStatus.set('disconnected');
      this.setStep('stopped');
      this.cleanup();
    });

    this.peer.on('close_abnormal', () => {
      if (this.currentStep === 'stopped') {
        log('Socket disconnected (abnormal) after normal close, ignoring');
        return;
      }

      log('Socket disconnected (abnormal)');
      connStatus.set('connecting');
      setStatus('Disconnected (abnormal)', 'error');
      this.setStep('unconnected');
      // do not cleanup to allow auto-reconnect to resume WebRTC session
      // this.cleanup();
    });

    // this.peer.on('close_invalid', (code) => {
    //   log(`Socket disconnected (invalid code: ${code})`, 'error');
    //   setStatus('Disconnected (error)', 'error');
    //   this.cleanup();
    // });
  }

  async joinRoom() {
    const log = this.log.bind(this);
    const setStatus = this.setStatus.bind(this);

    const resp = await this.peer.call('join', {
      pin: this.pin || undefined,
      clientId: get(clientId),
      jtoken: this.jtoken || undefined,
      displayName: this.displayName,
    });

    // TODO: do not add token if WebRTC session is closed
    if (resp?.jtoken) {
      this.jtoken = resp.jtoken;
      console.log('jtoken', this.jtoken);
      log('Received jtoken from server');
    }


    log('Joined room');
    setStatus('Joined room, starting WebRTC...', 'success');

    return resp?.resume;
  }

  async startWebRTC() {
    const log = this.log.bind(this);
    const setStatus = this.setStatus.bind(this);

    log('Creating PeerConnection...');

    this.pc = new window.RTCPeerConnection({
      iceServers: [],
    });

    this.localStream.getTracks().forEach(track => {
      this.pc.addTrack(track, this.localStream);
      log(`Added local track: ${track.kind}`);
    });

    this.pc.ontrack = (event) => {
      log(`Received remote track: ${event.track.kind}, enabled: ${event.track.enabled}, muted: ${event.track.muted}`);
      log(`Track readyState: ${event.track.readyState}`);
      log(`Number of streams: ${event.streams ? event.streams.length : 0}`);

      if (event.streams && event.streams[0]) {
        this.remoteStream = event.streams[0];
        log(`Remote audio stream attached. Stream active: ${event.streams[0].active}, tracks: ${event.streams[0].getTracks().length}`);

        event.track.onmute = () => log('Remote track muted!');
        event.track.onunmute = () => log('Remote track unmuted!');
        event.track.onended = () => log('Remote track ended!');

        // Show audio controls first
        showAudioCtrl.set(true);

        // Wait for next tick to ensure remoteAudioEl is bound
        setTimeout(() => {
          if (this.remoteAudioEl) {
            this.remoteAudioEl.srcObject = event.streams[0];
            this.remoteAudioEl.play().then(() => {
              log('Audio element playing successfully');
            }).catch(err => {
              log(`Audio play failed: ${err.message}`, 'error');
            });
          }
        }, 0);
      } else {
        log('WARNING: No streams in ontrack event!', 'error');
      }
    };

    this.pc.onconnectionstatechange = () => {
      log(`PeerConnection state: ${this.pc.connectionState}`);

      switch(this.pc.connectionState) {
        case 'connected':
          log('WebRTC connected');
          setStatus('WebRTC connected', 'success');
          break;
        case 'disconnected':
          log('WebRTC disconnected - network issue', 'error');
          setStatus('WebRTC disconnected - reconnecting...', 'error');
          break;
        case 'failed':
          log('WebRTC connection failed', 'error');
          setStatus('WebRTC connection failed', 'error');
          break;
        case 'closed':
          log('WebRTC connection closed');
          setStatus('WebRTC closed', 'info');
          break;
      }
    };

    this.pc.oniceconnectionstatechange = () => {
      log(`ICE connection state: ${this.pc.iceConnectionState}`);

      switch(this.pc.iceConnectionState) {
        case 'connected':
        case 'completed':
          log('ICE connected');
          break;
        case 'disconnected':
          log('ICE disconnected - may recover', 'error');
          break;
        case 'failed':
          log('ICE connection failed - need to restart', 'error');
          setStatus('ICE failed - connection lost', 'error');
          break;
        case 'closed':
          log('ICE closed');
          break;
      }
    };

    this.pc.onsignalingstatechange = () => {
      log(`Signaling state: ${this.pc.signalingState}`);
    };

    this.pc.onicecandidate = async (event) => {
      if (event.candidate) {
        log('Sending ICE candidate to server');
        await this.peer.notify('icecandidate', {
          candidate: event.candidate,
        });
      } else {
        log('ICE gathering complete');
      }
    };

    log('Creating WebRTC offer...');
    const offer = await this.pc.createOffer({
      offerToReceiveAudio: true,
      offerToReceiveVideo: false,
    });
    await this.pc.setLocalDescription(offer);

    log('Sending offer to server...');
    console.log('offer', offer);
    const resp = await this.peer.call('offer', {
      sdp: offer,
    });

    log('Received SDP answer from server');
    if (this.pc && resp.sdp) {
      await this.pc.setRemoteDescription(new window.RTCSessionDescription(resp.sdp));
      setStatus('Connected: Publishing and receiving audio', 'success');
    }
  }

  async run() {
    connStatus.set('connecting');
    this.setStep('unconnected');

    while (true) {
      try {
        const { next, nextDelay = 0 } = await this.runStepOnce();
        if (!next) break;
        await delay(nextDelay);
      } catch (err) {
        const msg = err.message || 'unknown error';
        this.log(`Error in step: ${msg}`);
        this.setStatus(`Error: ${msg}`, 'error');
        await delay(1000);
      }
    }
  }

  async runStepOnce() {
    // Step 1: Setup audio stream
    if (this.currentStep === 'unconnected') {
      await this.setupAudioStream();
      this.setStep('audio_ready');
    }

    // Step 2: Connect WebSocket
    if (this.currentStep === 'audio_ready') {
      await this.connect();
      this.setStep('connected');
    }

    // Step 3: Join room
    if (this.currentStep === 'connected') {
      const resume = await this.joinRoom();
      this.setStep(resume ? 'keepalive' :'joined');
    }

    // Step 4: Start WebRTC
    if (this.currentStep === 'joined') {
      await this.startWebRTC();
      this.log('WebRTC connected');
      this.setStep('offered');
    }

    // Step 5: Send initial keepalive and enter keepalive loop
    if (this.currentStep === 'offered') {
      const initialStatus = this.isPlaying() ? ANCHOR_STATUS.ONAIR : ANCHOR_STATUS.IDLE;
      await this.peer.notify('keepalive', { status: initialStatus });
      this.log(`Initial status sent: ${initialStatus}`);
      this.setStep('keepalive');
      return { nextDelay: KEEPALIVE_INTERVAL_MS, next: true };
    }

    // Step 6: Keepalive loop
    if (this.currentStep === 'keepalive') {
      const currentStatus = this.isPlaying() ? ANCHOR_STATUS.ONAIR : ANCHOR_STATUS.IDLE;
      await this.peer.notify('keepalive', { status: currentStatus });
      return { nextDelay: KEEPALIVE_INTERVAL_MS, next: true };
    }

    // Step 7: Stopped
    if (this.currentStep === 'stopped') {
      return { next: false };
    }

    throw new Error(`Unknown step: ${this.currentStep}`);
  }

  async disconnect() {
    const log = this.log.bind(this);
    const setStatus = this.setStatus.bind(this);

    log('Disconnecting...');
    setStatus('Disconnecting...', 'info');

    if (this.peer) {
      // TODO: cannot wait for leave, lead to socket abnormal close
      // await this.peer.notify('leave');
      await this.peer.close();
    }

    log('Successfully left the room');
    setStatus('Left the room', 'info');
    this.cleanup();
  }

  cleanup() {
    if (this.pc) {
      this.pc.close();
      this.pc = null;
    }

    if (this.localStream) {
      // this.localStream.getTracks().forEach(track => track.stop());
      this.localStream = null;
    }

    if (this.remoteStream) {
      // this.remoteStream.getTracks().forEach(track => track.stop());
      this.remoteStream = null;
    }

    if (this.remoteAudioEl) {
      this.remoteAudioEl.srcObject = null;
    }

    showAudioCtrl.set(false);

    if (this.peer) {
      this.peer.close();
      this.peer = null;
    }

    members.set([]);

    connStatus.set('disconnected');
    this.setStep('stopped');
  }

  updateConfig(config) {
    this.wsUrl = config.wsUrl ?? this.wsUrl;
    this.token = config.token ?? this.token;
    this.pin = config.pin ?? this.pin;
    this.displayName = config.displayName ?? this.displayName;
    this.wavUrl = config.wavUrl ?? this.wavUrl;
  }
}

function delay(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

export { KEEPALIVE_INTERVAL_MS, MAX_LOGS_SIZE };

// Accept HMR updates for this module
if (import.meta.hot) {
  import.meta.hot.accept();
}
