import type { Api } from '@/api/client';
import type { Conversation } from '@/api/types';
import { ChatsController } from '@/chats/controller';
import type { SocketLike } from '@/realtime/socket';

class FakeSocket implements SocketLike {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  close() {
    this.onclose?.();
  }
}

function conv(id: string, created_at: string): Conversation {
  return { id, kind: 'dm', participants: [], last_message: null, created_at };
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe('chats controller', () => {
  it('loads the list, applies live frames, and reloads on reconnect', async () => {
    let lists = 0;
    const api = {
      listConversations: async () => {
        lists += 1;
        return [conv('a', '2026-09-01T00:00:00Z')];
      },
    } as unknown as Api;
    const sockets: FakeSocket[] = [];
    const c = new ChatsController(api, 'ws://hub', 'ses_tok_1', () => {
      const s = new FakeSocket();
      sockets.push(s);
      return s;
    });
    const seen: number[] = [];
    c.subscribe(() => seen.push(c.getSnapshot().conversations.length));

    expect(c.getSnapshot().loading).toBe(true);
    c.start();
    await flush();
    expect(c.getSnapshot().loading).toBe(false);
    expect(c.getSnapshot().conversations.map((x) => x.id)).toEqual(['a']);
    expect(lists).toBe(1);

    sockets[0]?.onopen?.();
    expect(c.getSnapshot().connected).toBe(true);
    sockets[0]?.onmessage?.({
      data: JSON.stringify({
        type: 'message.created',
        data: {
          conversation_id: 'a',
          message: {
            id: 'm1',
            conversation_id: 'a',
            sender: { kind: 'agent', id: 'agt_1' },
            body: { text: 'hello' },
            reply_to: null,
            status: 'complete',
            truncated: false,
            delivery_status: null,
            created_at: '2026-09-05T00:00:00Z',
          },
        },
      }),
    });
    expect(c.getSnapshot().conversations[0]?.last_message?.body.text).toBe('hello');

    // A reconnect reloads from the hub: the record, not the frames.
    jest.useFakeTimers();
    sockets[0]?.onclose?.();
    expect(c.getSnapshot().connected).toBe(false);
    jest.advanceTimersByTime(1_000);
    jest.useRealTimers();
    sockets[1]?.onopen?.();
    await flush();
    expect(lists).toBe(2);

    c.stop();
    expect(seen.length).toBeGreaterThan(0);
  });

  it('keeps an error when the hub cannot be reached, and clears it on success', async () => {
    let fail = true;
    const api = {
      listConversations: async () => {
        if (fail) throw new Error('down');
        return [];
      },
    } as unknown as Api;
    const c = new ChatsController(api, 'ws://hub', 't', () => new FakeSocket());
    c.start();
    await flush();
    expect(c.getSnapshot().error).toBeInstanceOf(Error);
    expect(c.getSnapshot().loading).toBe(false);
    fail = false;
    await c.refresh();
    expect(c.getSnapshot().error).toBeNull();
    c.stop();
  });
});
