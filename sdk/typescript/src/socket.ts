/**
 * Receiving over a socket: the hub pushes events down a connection the
 * backend holds open, and the backend acknowledges each one.
 *
 * This is what one agent on a laptop uses — no public address, no TLS
 * certificate, nothing to deploy. A backend answering for many agents wants
 * webhooks instead: see `webhook.ts`.
 *
 * Operations can also travel up this socket (`typing`, `stream.*`) with a
 * correlation id the hub echoes back. Sending a message stays on HTTP, where
 * a failure has a status code and a body to explain it.
 */

/** Close codes that mean "do not come back". */
export const CLOSE_REPLACED = 4001;
export const CLOSE_BINDING_GONE = 4002;

/** How long to wait for the hub to answer an operation sent up the socket. */
const REPLY_TIMEOUT_MS = 30_000;

export interface SocketOptions {
  url: string;
  headers: Record<string, string>;
  onEvent: (envelope: unknown) => void;
  onOpen?: () => void;
  onClose?: (code: number, reason: string) => void;
  /** Swapped out in tests. Defaults to the global WebSocket. */
  WebSocketImpl?: typeof WebSocket;
}

interface Pending {
  resolve: (frame: Record<string, unknown>) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}

/**
 * One connection to the hub. It does not reconnect: `Agent` owns that policy,
 * because how long to wait and when to give up is a decision about the
 * backend, not about the socket.
 */
export class SocketSession {
  private socket?: WebSocket;
  private readonly pending = new Map<string, Pending>();
  private readonly options: SocketOptions;
  private closed = false;

  constructor(options: SocketOptions) {
    this.options = options;
  }

  /** Connect, and resolve when the hub closes the connection. */
  open(): Promise<{ code: number; reason: string }> {
    const Impl = this.options.WebSocketImpl ?? globalThis.WebSocket;
    // The header form of the constructor is Node's; browsers cannot send an
    // Authorization header on a WebSocket at all, which is why this SDK is a
    // server-side one.
    const socket = new Impl(this.options.url, {
      headers: this.options.headers,
    } as unknown as string[]) as WebSocket;
    this.socket = socket;

    return new Promise((resolve, reject) => {
      socket.addEventListener("open", () => this.options.onOpen?.());

      socket.addEventListener("message", (event: MessageEvent) => {
        let frame: Record<string, unknown>;
        try {
          frame = JSON.parse(String(event.data)) as Record<string, unknown>;
        } catch {
          return; // the hub does not send anything that is not JSON
        }
        this.route(frame);
      });

      socket.addEventListener("error", () => {
        // The close event follows and carries the reason; nothing to do here,
        // but without a listener Node treats it as an unhandled error.
      });

      socket.addEventListener("close", (event: CloseEvent) => {
        this.closed = true;
        this.failPending(new Error("the connection closed before the hub answered"));
        this.options.onClose?.(event.code, event.reason);
        resolve({ code: event.code, reason: event.reason });
      });

      // A handshake the hub refuses arrives as an error then a close, so the
      // promise above settles; this is only for a constructor that throws.
      if (!socket) reject(new Error("could not open a socket"));
    });
  }

  /** Route one frame: an answer to us, a rejection of ours, or an event. */
  private route(frame: Record<string, unknown>): void {
    const cid = frame.reply_to_cid;
    if (typeof cid === "string") {
      const waiting = this.pending.get(cid);
      if (waiting) {
        this.pending.delete(cid);
        clearTimeout(waiting.timer);
        waiting.resolve(frame);
      }
      return;
    }
    if (frame.id && frame.type) {
      this.options.onEvent(frame);
    }
  }

  /** Tell the hub an event was handled, so it stops redelivering it. */
  ack(eventId: string): void {
    this.sendRaw({ ack: eventId });
  }

  /** Send an operation and wait for the hub's answer. */
  call(op: string, fields: Record<string, unknown> = {}): Promise<Record<string, unknown>> {
    const cid = crypto.randomUUID();
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(cid);
        reject(new Error(`the hub did not answer ${op} in time`));
      }, REPLY_TIMEOUT_MS);
      this.pending.set(cid, {
        resolve: (frame) => {
          if (frame.ok === false) {
            const error = (frame.error ?? {}) as Record<string, string>;
            reject(new Error(`${error.code ?? "unknown"}: ${error.message ?? "the hub refused it"}`));
            return;
          }
          resolve(frame);
        },
        reject,
        timer,
      });
      try {
        this.sendRaw({ op, cid, ...fields });
      } catch (error) {
        this.pending.delete(cid);
        clearTimeout(timer);
        reject(error as Error);
      }
    });
  }

  private sendRaw(frame: Record<string, unknown>): void {
    if (!this.socket || this.closed) throw new Error("not connected to the hub");
    this.socket.send(JSON.stringify(frame));
  }

  close(): void {
    this.closed = true;
    this.socket?.close();
  }

  private failPending(error: Error): void {
    for (const waiting of this.pending.values()) {
      clearTimeout(waiting.timer);
      waiting.reject(error);
    }
    this.pending.clear();
  }
}
