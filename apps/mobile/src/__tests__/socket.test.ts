import { connectSocket, type SocketLike } from '@/realtime/socket';

// A fake socket the test drives by hand.
class FakeSocket implements SocketLike {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  closed = false;
  constructor(
    public url: string,
    public token: string,
  ) {}
  close() {
    this.closed = true;
    this.onclose?.();
  }
}

describe('live socket', () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it('sends the token, delivers frames, and reports open and close', () => {
    const made: FakeSocket[] = [];
    const frames: unknown[] = [];
    const opens: boolean[] = [];
    const handle = connectSocket({
      url: 'ws://hub/v1/client/socket',
      token: 'ses_tok_x',
      onFrame: (f) => frames.push(f),
      onOpen: (reconnect) => opens.push(reconnect),
      factory: (url, token) => {
        const s = new FakeSocket(url, token);
        made.push(s);
        return s;
      },
    });
    expect(made[0]?.token).toBe('ses_tok_x');
    made[0]?.onopen?.();
    made[0]?.onmessage?.({ data: JSON.stringify({ type: 'ready', data: { user_id: 'usr_1' } }) });
    made[0]?.onmessage?.({ data: 'not json' });
    expect(frames).toEqual([{ type: 'ready', data: { user_id: 'usr_1' } }]);
    expect(opens).toEqual([false]);
    handle.close();
    expect(made[0]?.closed).toBe(true);
    jest.advanceTimersByTime(60_000);
    expect(made).toHaveLength(1); // closed on purpose: no reconnect
  });

  it('reconnects with growing backoff and says so, so the caller catches up', () => {
    const made: FakeSocket[] = [];
    const opens: boolean[] = [];
    connectSocket({
      url: 'ws://hub',
      token: 't',
      onFrame: () => {},
      onOpen: (r) => opens.push(r),
      factory: (url, token) => {
        const s = new FakeSocket(url, token);
        made.push(s);
        return s;
      },
    });
    made[0]?.onopen?.();
    made[0]?.onclose?.(); // dropped at once
    expect(made).toHaveLength(1);
    jest.advanceTimersByTime(999);
    expect(made).toHaveLength(1);
    jest.advanceTimersByTime(1);
    expect(made).toHaveLength(2); // after 1s
    made[1]?.onclose?.();
    jest.advanceTimersByTime(1_999);
    expect(made).toHaveLength(2);
    jest.advanceTimersByTime(1);
    expect(made).toHaveLength(3); // after 2s
    made[2]?.onopen?.();
    expect(opens).toEqual([false, true]);

    // A connection that lasted resets the backoff.
    jest.advanceTimersByTime(11_000);
    made[2]?.onclose?.();
    jest.advanceTimersByTime(1_000);
    expect(made).toHaveLength(4);
  });
});
