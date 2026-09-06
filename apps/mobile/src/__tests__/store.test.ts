import type { Contact, Conversation, Message } from '@/api/types';
import {
  applyFrame,
  arrange,
  concernsUnknown,
  empty,
  filterConversations,
  setConversations,
} from '@/chats/store';

function msg(over: Partial<Message> & { id: string; conversation_id: string; created_at: string }): Message {
  return {
    sender: { kind: 'user', id: 'usr_1' },
    body: { text: 'hi' },
    reply_to: null,
    status: 'complete',
    truncated: false,
    delivery_status: 'pending',
    ...over,
  };
}

function conv(id: string, created_at: string, last: Message | null = null): Conversation {
  return { id, kind: 'dm', participants: [], last_message: last, created_at };
}

describe('chat list', () => {
  it('sorts by latest activity, newest first', () => {
    const s = setConversations(empty, [
      conv('cnv_old', '2026-09-01T10:00:00Z'),
      conv(
        'cnv_active',
        '2026-09-01T09:00:00Z',
        msg({ id: 'm', conversation_id: 'cnv_active', created_at: '2026-09-05T10:00:00Z' }),
      ),
      conv('cnv_new', '2026-09-03T10:00:00Z'),
    ]);
    expect(s.conversations.map((c) => c.id)).toEqual(['cnv_active', 'cnv_new', 'cnv_old']);
  });

  it('moves a conversation to the top when a message arrives', () => {
    let s = setConversations(empty, [conv('a', '2026-09-02T00:00:00Z'), conv('b', '2026-09-01T00:00:00Z')]);
    s = applyFrame(s, {
      type: 'message.created',
      data: {
        conversation_id: 'b',
        message: msg({ id: 'm1', conversation_id: 'b', created_at: '2026-09-05T00:00:00Z' }),
      },
    });
    expect(s.conversations[0]?.id).toBe('b');
    expect(s.conversations[0]?.last_message?.id).toBe('m1');
  });

  it('grows a streaming preview with deltas and finalises it', () => {
    let s = setConversations(empty, [conv('a', '2026-09-01T00:00:00Z')]);
    const started = msg({
      id: 'm1',
      conversation_id: 'a',
      created_at: '2026-09-05T00:00:00Z',
      status: 'streaming',
      body: {},
      sender: { kind: 'agent', id: 'agt_1' },
      delivery_status: null,
    });
    s = applyFrame(s, { type: 'message.started', data: { conversation_id: 'a', message: started } });
    s = applyFrame(s, {
      type: 'message.delta',
      data: { conversation_id: 'a', message_id: 'm1', text: 'I can ' },
    });
    s = applyFrame(s, {
      type: 'message.delta',
      data: { conversation_id: 'a', message_id: 'm1', text: 'see it.' },
    });
    expect(s.conversations[0]?.last_message?.body.text).toBe('I can see it.');
    s = applyFrame(s, {
      type: 'message.completed',
      data: {
        conversation_id: 'a',
        message: { ...started, status: 'complete', body: { text: 'I can see it.' } },
      },
    });
    expect(s.conversations[0]?.last_message?.status).toBe('complete');
  });

  it('updates the tick on the last message only', () => {
    const last = msg({ id: 'm1', conversation_id: 'a', created_at: '2026-09-05T00:00:00Z' });
    let s = setConversations(empty, [conv('a', '2026-09-01T00:00:00Z', last)]);
    s = applyFrame(s, {
      type: 'delivery.updated',
      data: { conversation_id: 'a', message_id: 'm1', delivery_status: 'delivered' },
    });
    expect(s.conversations[0]?.last_message?.delivery_status).toBe('delivered');
    s = applyFrame(s, {
      type: 'delivery.updated',
      data: { conversation_id: 'a', message_id: 'm0', delivery_status: 'failed' },
    });
    expect(s.conversations[0]?.last_message?.delivery_status).toBe('delivered');
  });

  it('ignores frames for conversations it does not know and older messages', () => {
    const last = msg({ id: 'm2', conversation_id: 'a', created_at: '2026-09-05T00:00:00Z' });
    const s0 = setConversations(empty, [conv('a', '2026-09-01T00:00:00Z', last)]);
    const s1 = applyFrame(s0, {
      type: 'message.created',
      data: {
        conversation_id: 'zzz',
        message: msg({ id: 'x', conversation_id: 'zzz', created_at: '2026-09-06T00:00:00Z' }),
      },
    });
    expect(s1).toBe(s0);
    const s2 = applyFrame(s0, {
      type: 'message.created',
      data: {
        conversation_id: 'a',
        message: msg({ id: 'm1', conversation_id: 'a', created_at: '2026-09-04T00:00:00Z' }),
      },
    });
    expect(s2.conversations[0]?.last_message?.id).toBe('m2');
  });
});

describe('search', () => {
  const list = [
    {
      ...conv(
        'cnv_a',
        '2026-09-01T10:00:00Z',
        msg({
          id: 'm1',
          conversation_id: 'cnv_a',
          created_at: '2026-09-02T10:00:00Z',
          body: { text: 'your refund is on its way' },
        }),
      ),
      participants: [{ kind: 'agent' as const, id: 'agt_1', display_name: 'SBI Support', handle: 'sbi' }],
    },
    {
      ...conv('cnv_b', '2026-09-01T10:00:00Z'),
      participants: [{ kind: 'agent' as const, id: 'agt_2', display_name: 'Echo', handle: 'echo-1' }],
    },
  ];

  it('matches names, handles and the last message, ignoring case', () => {
    expect(filterConversations(list, '').map((c) => c.id)).toEqual(['cnv_a', 'cnv_b']);
    expect(filterConversations(list, 'sbi').map((c) => c.id)).toEqual(['cnv_a']);
    expect(filterConversations(list, 'ECHO-').map((c) => c.id)).toEqual(['cnv_b']);
    expect(filterConversations(list, 'refund').map((c) => c.id)).toEqual(['cnv_a']);
    expect(filterConversations(list, 'nothing')).toEqual([]);
  });
});

describe('unknown conversations', () => {
  it('flags a frame about a chat the list has not seen, and nothing else', () => {
    const s = setConversations(empty, [conv('cnv_known', '2026-09-01T10:00:00Z')]);
    const frame = (conversation_id: string) =>
      ({
        type: 'delivery.updated' as const,
        data: { conversation_id, message_id: 'm', delivery_status: 'delivered' as const },
      }) as const;
    expect(concernsUnknown(s, frame('cnv_known'))).toBe(false);
    expect(concernsUnknown(s, frame('cnv_new'))).toBe(true);
    expect(concernsUnknown(s, { type: 'ready', data: { user_id: 'usr_1' } })).toBe(false);
    expect(concernsUnknown(s, { type: 'agent.status', data: { agent_id: 'a', status: 'idle' } })).toBe(false);
  });
});

describe('pinned and archived', () => {
  const withAgent = (id: string, agent: string, at: string): Conversation => ({
    ...conv(id, at),
    participants: [{ kind: 'agent', id: agent, display_name: agent }],
  });
  const decided = (agent: string, over: Partial<Contact>): Contact =>
    ({ agent: { id: agent }, pinned: false, archived: false, muted_until: null, ...over }) as Contact;

  it('lifts pinned chats above the rest and sets archived ones aside', () => {
    const list = setConversations(empty, [
      withAgent('c1', 'a1', '2026-09-05T00:00:00Z'),
      withAgent('c2', 'a2', '2026-09-04T00:00:00Z'),
      withAgent('c3', 'a3', '2026-09-03T00:00:00Z'),
      withAgent('c4', 'a4', '2026-09-02T00:00:00Z'),
    ]).conversations;
    const { shown, archived } = arrange(list, [
      decided('a3', { pinned: true }),
      decided('a4', { pinned: true }),
      decided('a2', { archived: true }),
    ]);
    // Pins keep their own activity order; the rest follow; archived are out.
    expect(shown.map((c) => c.id)).toEqual(['c3', 'c4', 'c1']);
    expect(archived.map((c) => c.id)).toEqual(['c2']);
  });

  it('leaves a chat with nothing decided where activity put it', () => {
    const list = [withAgent('c1', 'a1', '2026-09-05T00:00:00Z'), conv('g1', '2026-09-06T00:00:00Z')];
    expect(arrange(list, []).shown.map((c) => c.id)).toEqual(['c1', 'g1']);
  });
});
