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
  trimmed = false,
): Page => ({
  messages,
  next_before,
  next_after,
  trimmed,
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

  it('catches up after a reconnect from the newest message it has, and clears activity on time', async () => {
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
      type: 'activity',
      data: {
        conversation_id: 'cnv_1',
        agent_id: 'agt_1',
        state: 'working',
        label: 'Checking the weather',
        expires_at: '2026-09-05T10:00:12Z',
      },
    });
    expect(c.getSnapshot().activity?.label).toBe('Checking the weather');
    expect(c.getSnapshot().busy).toBe(true);
    now += 3_000;
    jest.advanceTimersByTime(3_000);
    expect(c.getSnapshot().activity).toBeNull();
    expect(c.getSnapshot().busy).toBe(false);
    jest.useRealTimers();
    c.stop();
  });

  it('tells the screen when the top of the chat is where the hub stopped keeping', async () => {
    const api = {
      listMessages: async (_id: string, q: { before?: string }) =>
        q.before
          ? page([msg('m01', 'first kept')], null, null, true)
          : page([msg('m02', 'hello')], 'm02', null, true),
    } as unknown as Api;
    const c = new ChatController(api, new FakeRealtime(), 'cnv_1', 'usr_1');
    c.start();
    await flush();
    // The hub said so from the first page, but there is more to load, so
    // the note waits until the oldest page is on screen.
    expect(c.getSnapshot().hasOlder).toBe(true);
    expect(c.getSnapshot().trimmed).toBe(false);
    await c.loadOlder();
    expect(c.getSnapshot().hasOlder).toBe(false);
    expect(c.getSnapshot().trimmed).toBe(true);
    c.stop();
  });

  it('stops the agent: the screen stops showing work at once, and the ended reply comes back stopped', async () => {
    const streaming: Message = { ...msg('m03', 'It is 31 and'), status: 'streaming' };
    let stops = 0;
    const api = {
      listMessages: async () => page([streaming, msg('m02', 'Weather?', 'user')]),
      stopConversation: async () => {
        stops += 1;
        return [{ ...streaming, status: 'complete', stopped: true, truncated: true }];
      },
    } as unknown as Api;
    const c = new ChatController(api, new FakeRealtime(), 'cnv_1', 'usr_1', () =>
      Date.parse('2026-09-05T10:00:04Z'),
    );
    c.start();
    await flush();
    expect(c.getSnapshot().writing).toBe(true);
    expect(c.getSnapshot().busy).toBe(true);

    await c.stopAgent();
    expect(stops).toBe(1);
    expect(c.getSnapshot().busy).toBe(false);
    expect(c.getSnapshot().messages[0]).toMatchObject({ id: 'm03', status: 'complete', stopped: true });
    c.stop();
  });

  it('marks the newest message read only while the person can see the chat', async () => {
    const reads: string[] = [];
    const api = {
      listMessages: async () => page([msg('m02', 'hello')]),
      markRead: async (_id: string, messageId: string) => {
        reads.push(messageId);
      },
    } as unknown as Api;
    const realtime = new FakeRealtime();
    const c = new ChatController(api, realtime, 'cnv_1', 'usr_1');
    jest.useFakeTimers();
    c.start();
    await jest.runAllTimersAsync();
    expect(reads).toEqual([]);

    c.setVisible(true);
    await jest.advanceTimersByTimeAsync(600);
    expect(reads).toEqual(['m02']);

    c.setVisible(false);
    realtime.emit({
      type: 'message.created',
      data: { conversation_id: 'cnv_1', message: msg('m04', 'later') },
    });
    await jest.advanceTimersByTimeAsync(600);
    expect(reads).toEqual(['m02']);

    c.setVisible(true);
    await jest.advanceTimersByTimeAsync(600);
    expect(reads).toEqual(['m02', 'm04']);
    jest.useRealTimers();
    c.stop();
  });

  it('offers to say it again once a received message has met thirty seconds of silence', async () => {
    let now = Date.parse('2026-09-05T10:00:05Z');
    const sent: unknown[] = [];
    const api = {
      listMessages: async () =>
        page([{ ...msg('m05', 'Weather in Goa?', 'user'), delivery_status: 'delivered' }]),
      sendMessage: async (_id: string, input: unknown) => {
        sent.push(input);
        return { ...msg('m09', 'Weather in Goa?', 'user') };
      },
    } as unknown as Api;
    jest.useFakeTimers();
    const c = new ChatController(api, new FakeRealtime(), 'cnv_1', 'usr_1', () => now);
    c.start();
    await jest.advanceTimersByTimeAsync(0);
    expect(c.getSnapshot().noReply).toBe(false);
    now += 31_000;
    await jest.advanceTimersByTimeAsync(31_000);
    expect(c.getSnapshot().noReply).toBe(true);
    expect(c.getSnapshot().lastWords).toBe('Weather in Goa?');

    await c.sendAgain();
    await jest.advanceTimersByTimeAsync(0);
    expect(sent).toEqual([expect.objectContaining({ text: 'Weather in Goa?' })]);
    jest.useRealTimers();
    c.stop();
  });
});
