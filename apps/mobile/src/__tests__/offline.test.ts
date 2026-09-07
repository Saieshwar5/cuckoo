import { NetworkError, type Api } from '@/api/client';
import type { Conversation, Message, Page } from '@/api/types';
import { guarded, keys, MemoryCache } from '@/cache/cache';
import { ChatController } from '@/chat/controller';
import { ChatsController } from '@/chats/controller';
import { Outbox, type OutboxEvent } from '@/outbox/outbox';

import { FakeRealtime, flush } from './fakes';

const conversation = (id: string): Conversation => ({
  id,
  kind: 'dm',
  participants: [{ kind: 'agent', id: 'agt_1', display_name: 'Echo', handle: 'echo' }],
  last_message: null,
  created_at: '2026-09-06T09:00:00Z',
});

const message = (id: string, text: string, kind: 'user' | 'agent' = 'agent'): Message => ({
  id,
  conversation_id: 'cnv_1',
  sender: { kind, id: kind === 'user' ? 'usr_1' : 'agt_1' },
  body: { text },
  reply_to: null,
  status: 'complete',
  truncated: false,
  delivery_status: kind === 'user' ? 'delivered' : null,
  created_at: `2026-09-06T09:00:${id.slice(-2)}Z`,
});

const page = (messages: Message[], trimmed = false): Page => ({
  messages,
  next_before: null,
  next_after: null,
  trimmed,
});

// never is a hub that cannot be reached: what an app with no signal sees.
const never = () => Promise.reject(new NetworkError(new TypeError('Network request failed')));

describe('cache', () => {
  it('round-trips documents and forgets on clear', async () => {
    const cache = new MemoryCache();
    await cache.set('a', { n: 1 });
    expect(await cache.get('a')).toEqual({ n: 1 });
    await cache.clear();
    expect(await cache.get('a')).toBeNull();
  });

  it('is never the reason a screen fails', async () => {
    const broken = {
      get: async () => {
        throw new Error('disk full');
      },
      set: async () => {
        throw new Error('disk full');
      },
      remove: async () => {
        throw new Error('disk full');
      },
      clear: async () => {
        throw new Error('disk full');
      },
    };
    const errors: unknown[] = [];
    const cache = guarded(broken, (e) => errors.push(e));
    expect(await cache.get('a')).toBeNull();
    await expect(cache.set('a', 1)).resolves.toBeUndefined();
    expect(errors).toHaveLength(2);
  });
});

describe('the chat list offline', () => {
  it('shows what it remembered before the hub answers, then what the hub says', async () => {
    const cache = new MemoryCache();
    await cache.set(keys.conversations('usr_1'), [conversation('cnv_old')]);

    let answer: (list: Conversation[]) => void = () => {};
    const api = {
      listConversations: () => new Promise<Conversation[]>((r) => (answer = r)),
    } as unknown as Api;
    const c = new ChatsController(api, new FakeRealtime(), { cache, userId: 'usr_1' });
    c.start();
    await flush();

    // Remembered, on screen, and not loading: there is something to look at.
    expect(c.getSnapshot().conversations.map((x) => x.id)).toEqual(['cnv_old']);
    expect(c.getSnapshot().loading).toBe(false);

    answer([conversation('cnv_new'), conversation('cnv_old')]);
    await flush();
    expect(c.getSnapshot().conversations.map((x) => x.id)).toEqual(['cnv_new', 'cnv_old']);
    // And what the hub said is what is remembered now.
    expect(((await cache.get(keys.conversations('usr_1'))) as Conversation[]).map((x) => x.id)).toEqual([
      'cnv_new',
      'cnv_old',
    ]);
  });

  it('keeps the remembered list when the hub cannot be reached', async () => {
    const cache = new MemoryCache();
    await cache.set(keys.conversations('usr_1'), [conversation('cnv_old')]);
    const api = { listConversations: never } as unknown as Api;
    const c = new ChatsController(api, new FakeRealtime(), { cache, userId: 'usr_1' });
    c.start();
    await flush();
    expect(c.getSnapshot().conversations).toHaveLength(1);
    expect(c.getSnapshot().error).toBeInstanceOf(NetworkError);
  });
});

describe('the outbox', () => {
  function setup(api: Partial<Api>, cache = new MemoryCache()) {
    const realtime = new FakeRealtime();
    const outbox = new Outbox(api as Api, cache, realtime, 'usr_1');
    const events: OutboxEvent[] = [];
    outbox.subscribe((e) => events.push(e));
    outbox.start();
    return { outbox, realtime, cache, events };
  }

  it('holds a send while the hub is unreachable and delivers it, in order, when the hub is back', async () => {
    let reachable = false;
    const sent: string[] = [];
    const api: Partial<Api> = {
      sendMessage: async (_id, input) => {
        if (!reachable) throw new NetworkError(new Error('offline'));
        sent.push(input.text ?? '');
        return message(`m0${sent.length}`, input.text ?? '', 'user');
      },
    };
    const { outbox, realtime, cache, events } = setup(api);
    await outbox.enqueue('k1', 'cnv_1', { text: 'first' });
    await outbox.enqueue('k2', 'cnv_1', { text: 'second' });
    await flush();

    expect(sent).toEqual([]);
    expect(outbox.pending('cnv_1').map((i) => i.key)).toEqual(['k1', 'k2']);
    // Written down, so closing the app loses nothing.
    expect(await cache.get(keys.outbox('usr_1'))).toHaveLength(2);
    // Nothing was reported: a send the hub could not be reached for is
    // still pending, which is the truth of it.
    expect(events.filter((e) => e.type !== 'queued')).toEqual([]);

    reachable = true;
    realtime.open(true);
    await flush();
    await flush();
    expect(sent).toEqual(['first', 'second']);
    expect(outbox.pending('cnv_1')).toEqual([]);
    expect(events.filter((e) => e.type === 'sent')).toHaveLength(2);
    expect(await cache.get(keys.outbox('usr_1'))).toEqual([]);
    outbox.stop();
  });

  it('sends the moment the platform says the network is back', async () => {
    let reachable = false;
    let networkBack: () => void = () => {};
    const api: Partial<Api> = {
      sendMessage: async (_id, input) => {
        if (!reachable) throw new NetworkError(new Error('offline'));
        return message('m01', input.text ?? '', 'user');
      },
    };
    const realtime = new FakeRealtime();
    const outbox = new Outbox(api as Api, new MemoryCache(), realtime, 'usr_1', (l) => {
      networkBack = l;
      return () => {};
    });
    const events: OutboxEvent[] = [];
    outbox.subscribe((e) => events.push(e));
    outbox.start();
    await outbox.enqueue('k1', 'cnv_1', { text: 'from the tunnel' });
    expect(events.filter((e) => e.type === 'sent')).toHaveLength(0);

    // Out of the tunnel. No socket yet, no timer due: the platform said so.
    reachable = true;
    networkBack();
    await flush();
    await flush();
    expect(events.filter((e) => e.type === 'sent')).toHaveLength(1);
    outbox.stop();
  });

  it('marks a send the hub refused as failed, and retries it when asked', async () => {
    let refuse = true;
    const api: Partial<Api> = {
      sendMessage: async (_id, input) => {
        if (refuse) throw new Error('422 invalid_text');
        return message('m01', input.text ?? '', 'user');
      },
    };
    const { outbox, events } = setup(api);
    await outbox.enqueue('k1', 'cnv_1', { text: 'x' });
    expect(events.at(-1)?.type).toBe('failed');
    expect(outbox.pending('cnv_1')[0]?.state).toBe('failed');

    refuse = false;
    await outbox.retry('k1');
    expect(events.at(-1)?.type).toBe('sent');
    expect(outbox.pending('cnv_1')).toEqual([]);
    outbox.stop();
  });

  it('picks up where a closed app left off, without uploading a photo twice', async () => {
    const cache = new MemoryCache();
    // What the last run wrote down: a photo already uploaded, message never sent.
    await cache.set(keys.outbox('usr_1'), [
      {
        key: 'k1',
        conversationId: 'cnv_1',
        request: {
          text: 'look',
          files: [
            { uri: 'file:///p.jpg', name: 'p.jpg', mimeType: 'image/jpeg', kind: 'image', byteSize: 1 },
          ],
        },
        uploaded: ['med_1'],
        state: 'queued',
        createdAt: '2026-09-06T09:00:00Z',
      },
    ]);
    let uploads = 0;
    const sends: unknown[] = [];
    const api: Partial<Api> = {
      uploadMedia: async () => {
        uploads++;
        throw new Error('should not upload again');
      },
      sendMessage: async (_id, input) => {
        sends.push(input);
        return message('m01', 'look', 'user');
      },
    };
    const { outbox } = setup(api, cache);
    await flush();
    await flush();
    expect(uploads).toBe(0);
    expect(sends).toEqual([{ text: 'look', attachments: ['med_1'], action: undefined, reply_to: undefined }]);
    outbox.stop();
  });
});

describe('one chat offline', () => {
  it('opens from memory with its unsent messages, then takes the page from the hub', async () => {
    const cache = new MemoryCache();
    await cache.set(keys.messages('usr_1', 'cnv_1'), {
      messages: [message('m02', 'remembered'), message('m01', 'older')],
      nextBefore: null,
      trimmed: true,
    });
    await cache.set(keys.outbox('usr_1'), [
      {
        key: 'k9',
        conversationId: 'cnv_1',
        request: { text: 'typed in a tunnel' },
        uploaded: [],
        state: 'queued',
        createdAt: '2026-09-06T09:00:09Z',
      },
    ]);
    let answer: (p: Page) => void = () => {};
    const api = {
      listMessages: () => new Promise<Page>((r) => (answer = r)),
      sendMessage: never,
    } as unknown as Api;
    const realtime = new FakeRealtime();
    const outbox = new Outbox(api, cache, realtime, 'usr_1');
    outbox.start();
    const c = new ChatController(api, realtime, 'cnv_1', 'usr_1', Date.now, {
      cache,
      userId: 'usr_1',
      outbox,
    });
    c.start();
    await flush();
    await flush();

    const texts = () => c.getSnapshot().messages.map((m) => m.body.text);
    expect(texts()).toEqual(['typed in a tunnel', 'remembered', 'older']);
    expect(c.getSnapshot().messages[0]?.delivery_status).toBe('pending');
    expect(c.getSnapshot().loading).toBe(false);
    // Remembered along with the messages: the note about older history
    // is there offline too.
    expect(c.getSnapshot().trimmed).toBe(true);

    // The hub's page replaces what was remembered; what we have not sent
    // yet is ours and stays.
    answer(page([message('m03', 'newest'), message('m02', 'remembered'), message('m01', 'older')]));
    await flush();
    expect(texts()).toEqual(['typed in a tunnel', 'newest', 'remembered', 'older']);
    expect(c.getSnapshot().trimmed).toBe(false);
    c.stop();
    outbox.stop();
  });

  it('writes what the hub sent down, without our unsent messages', async () => {
    jest.useFakeTimers();
    try {
      const cache = new MemoryCache();
      const api = {
        listMessages: async () => page([message('m01', 'hello')], true),
        sendMessage: never,
      } as unknown as Api;
      const realtime = new FakeRealtime();
      const outbox = new Outbox(api, cache, realtime, 'usr_1');
      const c = new ChatController(api, realtime, 'cnv_1', 'usr_1', Date.now, {
        cache,
        userId: 'usr_1',
        outbox,
      });
      c.start();
      await jest.advanceTimersByTimeAsync(10);
      await c.send({ text: 'unsent' });
      await jest.advanceTimersByTimeAsync(1000);

      const saved = (await cache.get(keys.messages('usr_1', 'cnv_1'))) as {
        messages: Message[];
        trimmed: boolean;
      };
      expect(saved.messages.map((m) => m.body.text)).toEqual(['hello']);
      expect(saved.trimmed).toBe(true);
      c.stop();
      outbox.stop();
    } finally {
      jest.useRealTimers();
    }
  });
});
