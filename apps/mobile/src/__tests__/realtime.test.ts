import { createRealtime } from '@/realtime/realtime';
import type { SocketLike } from '@/realtime/socket';

class FakeSocket implements SocketLike {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  closed = false;
  close() {
    this.closed = true;
  }
}

describe('shared live connection', () => {
  it('hands every frame to every subscriber and tracks the connection', () => {
    const sockets: FakeSocket[] = [];
    const rt = createRealtime({
      url: 'ws://hub',
      token: 't',
      factory: () => {
        const s = new FakeSocket();
        sockets.push(s);
        return s;
      },
    });
    const a: string[] = [];
    const b: string[] = [];
    const opens: boolean[] = [];
    rt.subscribe({ onFrame: (f) => a.push(f.type), onOpen: (r) => opens.push(r) });
    const stopB = rt.subscribe({ onFrame: (f) => b.push(f.type) });

    expect(rt.connected).toBe(false);
    rt.start();
    sockets[0]?.onopen?.();
    expect(rt.connected).toBe(true);
    expect(opens).toEqual([false]);

    sockets[0]?.onmessage?.({ data: JSON.stringify({ type: 'ready', data: { user_id: 'usr_1' } }) });
    expect(a).toEqual(['ready']);
    expect(b).toEqual(['ready']);

    stopB();
    sockets[0]?.onmessage?.({ data: JSON.stringify({ type: 'ready', data: { user_id: 'usr_1' } }) });
    expect(a).toEqual(['ready', 'ready']);
    expect(b).toEqual(['ready']);

    rt.stop();
    expect(sockets[0]?.closed).toBe(true);
    expect(rt.connected).toBe(false);
  });

  it('treats coming back from the background as a reconnect, so listeners catch up', () => {
    const sockets: FakeSocket[] = [];
    const rt = createRealtime({
      url: 'ws://hub',
      token: 't',
      factory: () => {
        const s = new FakeSocket();
        sockets.push(s);
        return s;
      },
    });
    const opens: boolean[] = [];
    let closes = 0;
    rt.subscribe({ onFrame: () => {}, onOpen: (r) => opens.push(r), onClose: () => (closes += 1) });

    rt.start();
    sockets[0]?.onopen?.();
    // Into the background: the hub must stop believing anyone is looking.
    rt.stop();
    expect(sockets[0]?.closed).toBe(true);
    expect(closes).toBe(1);
    // Back to the front: a new socket, and it counts as a reconnect.
    rt.start();
    sockets[1]?.onopen?.();
    expect(opens).toEqual([false, true]);
    rt.stop();
  });
});
