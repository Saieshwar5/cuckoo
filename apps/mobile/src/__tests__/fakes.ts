import type { Frame } from '@/api/types';
import type { Realtime, Subscriber } from '@/realtime/realtime';

// FakeRealtime is the live connection as a test drives it by hand.
export class FakeRealtime implements Realtime {
  connected = false;
  subs = new Set<Subscriber>();

  subscribe(sub: Subscriber): () => void {
    this.subs.add(sub);
    return () => {
      this.subs.delete(sub);
    };
  }

  open(reconnect = false): void {
    this.connected = true;
    for (const s of this.subs) s.onOpen?.(reconnect);
  }

  close(): void {
    this.connected = false;
    for (const s of this.subs) s.onClose?.();
  }

  emit(frame: Frame): void {
    for (const s of this.subs) s.onFrame(frame);
  }
}

export const flush = () => new Promise((r) => setTimeout(r, 0));
