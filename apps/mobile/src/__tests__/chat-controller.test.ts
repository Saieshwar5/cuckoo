import type { Api } from '@/api/client';
import type { Message, Page } from '@/api/types';
import { ChatController } from '@/chat/controller';

import { FakeRealtime, flush } from './fakes';

function msg(id: string, text: string, kind: 'user' | 'agent' = 'agent'): Message {
  return {
    id,
    conversation_id: 'cnv_1',
    sender: { kind, id: kind === 'user' ? 'usr_1' : 'agt_1' },
    body: { text },
    reply_to: null,
    status: 'complete',
    truncated: false,
    delivery_status: kind === 'user' ? 'pending' : null,
    created_at: `2026-09-05T10:00:${id.slice(-2)}Z`,
  };
}

const page = (
  messages: Message[],
  next_before: string | null = null,
  next_after: string | null = null,
): Page => ({
  messages,
  next_before,
  next_after,
});

describe('chat controller', () => {
  it('loads history, shows a send at once, confirms it, and retries a failure with the same key', async () => {
    const sent: { input: unknown; key: string }[] = [];
    let fail = false;
    const api = {
      listMessages: async () => page([msg('m02', 'hello')]),
      sendMessage: async (_id: string, input: unknown, key: string) => {
        sent.push({ input, key });
        if (fail) throw new Error('down');
        return { ...msg('m03', 'hi there', 'user'), created_at: '2026-09-05T10:00:03Z' };
      },
    } as unknown as Api;
    const c = new ChatController(api, new FakeRealtime(), 'cnv_1', 'usr_1', () =>
      Date.parse('2026-09-05T10:00:03Z'),
    );
    c.start();
    await flush();
    expect(c.getSnapshot().loading).toBe(false);
    expect(c.getSnapshot().messages.map((m) => m.id)).toEqual(['m02']);

    fail = true;
    const sending = c.send({ text: 'hi there' });
    expect(c.getSnapshot().messages[0]?.localKey).toBeDefined();
    expect(c.getSnapshot().messages[0]?.delivery_status).toBe('pending');
    await sending;
    expect(c.getSnapshot().messages[0]?.delivery_status).toBe('failed');

    fail = false;
    await c.retry(c.getSnapshot().messages[0]!.localKey!);
    expect(sent.map((s) => s.key)).toEqual([sent[0]!.key, sent[0]!.key]);
    expect(c.getSnapshot().messages.map((m) => m.id)).toEqual(['m03', 'm02']);
    expect(c.getSnapshot().messages[0]?.localKey).toBeUndefined();
    c.stop();
  });

  it('catches up after a reconnect from the newest message it has, and clears typing on time', async () => {
    const asked: unknown[] = [];
    const api = {
      listMessages: async (_id: string, q: unknown) => {
        asked.push(q);
        if (asked.length === 1) return page([msg('m02', 'hello')]);
        return page([msg('m05', 'missed you')]);
      },
    } as unknown as Api;
    const realtime = new FakeRealtime();
    let now = Date.parse('2026-09-05T10:00:10Z');
    const c = new ChatController(api, realtime, 'cnv_1', 'usr_1', () => now);
    c.start();
    await flush();
    realtime.open(true);
    await flush();
    expect(asked[1]).toEqual({ after: 'm02', limit: 100 });
    expect(c.getSnapshot().messages.map((m) => m.id)).toEqual(['m05', 'm02']);

    jest.useFakeTimers();
    realtime.emit({
      type: 'typing',
      data: {
        conversation_id: 'cnv_1',
        agent_id: 'agt_1',
        state: 'start',
        expires_at: '2026-09-05T10:00:12Z',
      },
    });
    expect(c.getSnapshot().typing).toBe(true);
    now += 3_000;
    jest.advanceTimersByTime(3_000);
    expect(c.getSnapshot().typing).toBe(false);
    jest.useRealTimers();
    c.stop();
  });
});
