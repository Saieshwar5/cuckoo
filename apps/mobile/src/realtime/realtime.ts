import type { Frame } from '../api/types';
import { connectSocket, type SocketFactory, type SocketHandle } from './socket';

export interface Subscriber {
  onFrame: (frame: Frame) => void;
  // Called every time the connection is (re)established.
  onOpen?: (reconnect: boolean) => void;
  onClose?: () => void;
}

// Realtime is the one live connection a signed-in session holds, shared by
// everything that wants frames: the chat list, the open chat. Each reads
// the frames it cares about and ignores the rest.
export interface Realtime {
  subscribe(sub: Subscriber): () => void;
  readonly connected: boolean;
}

export interface RealtimeHandle extends Realtime {
  start(): void;
  stop(): void;
  // The network is back: connect now rather than after the backoff.
  retryNow(): void;
}

export function createRealtime(opts: {
  url: string;
  token: string;
  factory?: SocketFactory;
}): RealtimeHandle {
  const subs = new Set<Subscriber>();
  let socket: SocketHandle | null = null;
  let connected = false;
  return {
    get connected() {
      return connected;
    },
    subscribe(sub) {
      subs.add(sub);
      return () => {
        subs.delete(sub);
      };
    },
    start() {
      if (socket) return;
      socket = connectSocket({
        url: opts.url,
        token: opts.token,
        factory: opts.factory,
        onFrame: (frame) => {
          for (const s of subs) s.onFrame(frame);
        },
        onOpen: (reconnect) => {
          connected = true;
          for (const s of subs) s.onOpen?.(reconnect);
        },
        onClose: () => {
          connected = false;
          for (const s of subs) s.onClose?.();
        },
      });
    },
    stop() {
      socket?.close();
      socket = null;
      connected = false;
    },
    retryNow() {
      socket?.retryNow();
    },
  };
}
