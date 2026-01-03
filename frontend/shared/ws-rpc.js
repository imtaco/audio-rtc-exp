import { moduleLogger } from './logger.js';
import { Client }from 'rpc-websockets'

const logger = moduleLogger('WSRPC');
// const CONNECT_TIMEOUT = 10_000; // 10 seconds
// const RECONNECT_ATTEMPTS = 30;

export class WSRpcClient {
  constructor(url, token) {
    this.ws = new Client(
      `${url}?token=${token}`,
      {
        // manually reconnect to control the flow
        autoconnect: false,
        reconnect: false,
      },
    );
  }

  connect() {
    this.ws.connect();

    return new Promise((resolve, reject) => {
      this.ws.once('open', () => {
        this.ws.off('error');
        this.registerEvents();
        resolve();
      });
      this.ws.once('error', err => {
        reject(err);
      });
    });
  }

  async call(method, params) {
    return await this.ws.call(method, params);
  }

  async notify(method, params) {
    return await this.ws.notify(method, params);
  }

  async close() {
    this.ws.close();
  }

  registerEvents() {
    // console.log('register ws rpc events');
    this.ws.on('error', (err) => {
      this.ws.emit('error', err);
    });
    this.ws.on('close', (code, reason) => {
      if (code === 1000) {
        this.ws.emit('close_normal');
      } else if (code === 1006 || code === 1005) {
        this.ws.emit('close_abnormal');
      } else {
        this.ws.emit('close_invalid', code);
      }

      logger.info({ code, reason }, 'WS RPC closed');
    });
  }

  def(method, handler) {
    this.ws.subscribe(method);
    this.ws.on(method, handler);
  }

  // on error
  // on close
  on(event, handler) {
    this.ws.on(event, handler);
  }

  off(event, handler) {
    this.ws.off(event, handler);
  }
}

export default WSRpcClient;