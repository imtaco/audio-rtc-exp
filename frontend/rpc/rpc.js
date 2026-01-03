import { writable } from 'svelte/store';
import { WSRpcClient } from '../shared/ws-rpc.js';

// Constants
const MAX_LOGS_SIZE = 100;

// Svelte stores for reactive state
export const clientId = writable(window.crypto.randomUUID().toString());
export const logs = writable([]);
export const statusMessage = writable('');
export const statusType = writable('info');
export const currentStep = writable('unconnected');
export const isConnected = writable(false);

export class RPCClient {
  constructor(config = {}) {
    clientId.set(config.clientId || window.crypto.randomUUID().toString());

    this.wsUrl = config.wsUrl || 'ws://localhost:8081/ws';
    this.token = config.token || '';
    this.currentStep = 'unconnected';

    // State
    this.peer = null;
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

  async connect() {
    const log = this.log.bind(this);
    const setStatus = this.setStatus.bind(this);

    try {
      log(`Connecting to ${this.wsUrl}...`);
      setStatus('Connecting...', 'info');

      this.peer = new WSRpcClient(this.wsUrl, this.token);
      await this.peer.connect();

      log('Connected to WebSocket');
      setStatus('Connected', 'success');

      this.peer.on('error', (error) => {
        log(`Socket error: ${error}`);
        setStatus('Connection error', 'error');
      });

      this.peer.on('close_normal', () => {
        log('Socket disconnected (normal)');
        setStatus('Disconnected', 'info');
        this.setStep('stopped');
        this.cleanup();
      });

      this.peer.on('close_abnormal', () => {
        log('Socket disconnected (abnormal)');
        setStatus('Disconnected (abnormal)', 'error');
        this.setStep('unconnected');
        this.cleanup();
      });

      // this.peer.on('close_invalid', (code) => {
      //   log(`Socket disconnected (invalid code: ${code})`, 'error');
      //   setStatus('Disconnected (error)', 'error');
      //   this.cleanup();
      // });

      isConnected.set(true);

    } catch (error) {
      log(`Error: ${error.message}`);
      setStatus(`Error: ${error.message}`, 'error');
      this.cleanup();
      throw error;
    }
  }

  async disconnect() {
    const log = this.log.bind(this);
    const setStatus = this.setStatus.bind(this);

    log('Disconnecting...');
    setStatus('Disconnecting...', 'info');

    if (this.peer) {
      await this.peer.close();
    }

    log('Disconnected');
    setStatus('Disconnected', 'info');
    this.setStep('stopped');
    this.cleanup();
  }

  cleanup() {
    // setConnState(false)

    if (this.peer) {
      this.peer.close();
      this.peer = null;
    }
    isConnected.set(false);
  }

  updateConfig(config) {
    this.wsUrl = config.wsUrl ?? this.wsUrl;
    this.token = config.token ?? this.token;
  }

  async run() {
    while (true) {
      try {
        const { next, nextDelay = 0 } = await this.runStepOnce();
        if (!next) break;
        await delay(nextDelay);
      } catch (err) {
        this.log(`Error in step: ${err.message}`);
        await delay(1000);
      }
    }
  }

  async runStepOnce() {
    if (this.currentStep === 'unconnected') {
      await this.connect();
      this.setStep('connected');
    }

    if (this.currentStep === 'connected') {
      // further steps can be added here
      await this.peer.call('add', { a: 1, b: 2 });
      this.setStep('joined');
    }

    if (this.currentStep === 'joined') {
      // further steps can be added here
      await this.peer.call('add', { a: 3, b: 4 });
      this.setStep('offered');
    }

    if (this.currentStep === 'offered' || this.currentStep === 'keepalive') {
      await this.peer.call('add', { a: 5, b: 6 });
      this.setStep('keepalive');
      return { nextDelay: 8_000, next: true }
    }

    if (this.currentStep === 'stopped') {
      return { next: false }
    }

    throw new Error(`Unknown step: ${this.currentStep}`);
  }
}

function delay(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

export { MAX_LOGS_SIZE };

// Accept HMR updates for this module
if (import.meta.hot) {
  import.meta.hot.accept();
}
