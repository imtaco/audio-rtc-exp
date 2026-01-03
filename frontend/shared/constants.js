// Room status constants
export const ROOM_STATUS = Object.freeze({
  ONAIR: 'onair',
  REMOVING: 'removing',
});

export const ANCHOR_STATUS = Object.freeze({
  ONAIR: 'onair',
  IDLE: 'idle',
  LEFT: 'left',
});

export function isRoomOnAir(state) {
  return state?.[ROOM_STATUS.LIVEMETA]?.status === ROOM_STATUS.ONAIR;
}

// WS-RPC Peer events
export const WS_RPC_EVENTS = Object.freeze({
  CONNECTED: 'connected',
  PEER_CLOSE: 'peer_closed',
  CONNECTION_LOST: 'connection_lost',
  CONNECT_ERROR: 'connect_error',
  RECONNECTED: 'reconnected',
  RECONNECT_ATTEMPT: 'reconnect_attempt',
  RECONNECT_FAILED: 'reconnect_failed',
  ERROR: 'error',
  CLOSE: 'close',
});

