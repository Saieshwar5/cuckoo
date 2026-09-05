import type { Frame } from '../api/types';

// A minimal WebSocket shape, so tests can supply their own.
export interface SocketLike {
  onopen: (() => void) | null;
  onmessage: ((ev: { data: unknown }) => void) | null;
  onclose: (() => void) | null;
  onerror: ((ev: unknown) => void) | null;
  close(): void;
}

export type SocketFactory = (url: string, token: string) => SocketLike;

export interface SocketOptions {
  url: string;
  token: string;
  onFrame: (frame: Frame) => void;
  // Called every time the connection is (re)established. The caller catches
  // up from the hub here: the socket is a hint, the hub is the record.
  onOpen?: (reconnect: boolean) => void;
  onClose?: () => void;
  factory?: SocketFactory;
  maxBackoffMs?: number;
}

export interface SocketHandle {
  close(): void;
}

// The token travels as a WebSocket subprotocol: "cuckoo" plus the token.
// Browsers cannot set headers on a WebSocket and this is the one thing they
// let a page send; it works the same on a phone, so there is one path.
const nativeFactory: SocketFactory = (url, token) =>
  new WebSocket(url, ['cuckoo', token]) as unknown as SocketLike;

// connectSocket holds one live-update connection and reconnects with
// backoff when it drops. A connection that lasted a while earns a fresh
// backoff, so a flaky network is retried quickly and a dead hub is not
// hammered.
export function connectSocket(opts: SocketOptions): SocketHandle {
  const factory = opts.factory ?? nativeFactory;
  const maxBackoff = opts.maxBackoffMs ?? 30_000;
  let backoff = 1_000;
  let closed = false;
  let opens = 0;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let current: SocketLike | null = null;

  const connect = () => {
    if (closed) return;
    const openedAt = Date.now();
    const ws = factory(opts.url, opts.token);
    current = ws;

    ws.onopen = () => {
      opens += 1;
      opts.onOpen?.(opens > 1);
    };
    ws.onmessage = (ev) => {
      try {
        opts.onFrame(JSON.parse(String(ev.data)) as Frame);
      } catch {
        // A frame we cannot read is a bug on one side; not a reason to drop
        // the connection.
      }
    };
    ws.onerror = () => {
      // onclose follows; nothing to do here.
    };
    ws.onclose = () => {
      if (current !== ws) return;
      current = null;
      opts.onClose?.();
      if (closed) return;
      if (Date.now() - openedAt > 10_000) backoff = 1_000;
      timer = setTimeout(connect, backoff);
      backoff = Math.min(backoff * 2, maxBackoff);
    };
  };

  connect();

  return {
    close() {
      closed = true;
      if (timer) clearTimeout(timer);
      const ws = current;
      current = null;
      ws?.close();
    },
  };
}
