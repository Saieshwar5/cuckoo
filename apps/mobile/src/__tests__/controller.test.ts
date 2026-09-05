import type { Api } from '@/api/client';
import type { Conversation } from '@/api/types';
import { ChatsController } from '@/chats/controller';

import { FakeRealtime, flush } from './fakes';

function conv(id: string, created_at: string): Conversation {
  return { id, kind: 'dm', participants: [], last_message: null, created_at };
}

describe('chats controller', () => {
  it('loads the list, applies live frames, and reloads on reconnect', async () => {
    let lists = 0;
    const api = {
      listConversations: async () => {
        lists += 1;
        return [conv('a', '2026-09-01T00:00:00Z')];
      },
    } as unknown as Api;
    const realtime = new FakeRealtime();
    const c = new ChatsController(api, realtime);
    const seen: number[] = [];
    c.subscribe(() => seen.push(c.getSnapshot().conversations.length));

    expect(c.getSnapshot().loading).toBe(true);
    c.start();
    await flush();
    expect(c.getSnapshot().loading).toBe(false);
    expect(c.getSnapshot().conversations.map((x) => x.id)).toEqual(['a']);
    expect(lists).toBe(1);

    realtime.open();
    expect(c.getSnapshot().connected).toBe(true);
    realtime.emit({
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
    });
    expect(c.getSnapshot().conversations[0]?.last_message?.body.text).toBe('hello');

    // A reconnect reloads from the hub: the record, not the frames.
    realtime.close();
    expect(c.getSnapshot().connected).toBe(false);
    realtime.open(true);
    await flush();
    expect(lists).toBe(2);

    c.stop();
    expect(realtime.subs.size).toBe(0);
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
    const c = new ChatsController(api, new FakeRealtime());
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
