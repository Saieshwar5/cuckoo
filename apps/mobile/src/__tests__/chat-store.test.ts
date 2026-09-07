import type { Message } from '@/api/types';
import {
  addLocal,
  addOlder,
  appendDelta,
  applyFrame,
  clearMessages,
  confirmLocal,
  empty,
  failLocal,
  historyTrimmed,
  isTyping,
  newestServerId,
  quickReplies,
  removeMessage,
  setPage,
  unseenSince,
  upsert,
  type ChatMessage,
} from '@/chat/store';

function msg(over: Partial<Message> & { id: string; created_at: string }): Message {
  return {
    conversation_id: 'cnv_1',
    sender: { kind: 'agent', id: 'agt_1' },
    body: { text: over.id },
    reply_to: null,
    status: 'complete',
    truncated: false,
    delivery_status: null,
    ...over,
  };
}

const t = (s: number) => `2026-09-05T10:00:${String(s).padStart(2, '0')}Z`;

describe('chat store', () => {
  it('keeps messages newest first through paging and inserts', () => {
    let s = setPage(empty, {
      messages: [msg({ id: 'm3', created_at: t(3) }), msg({ id: 'm2', created_at: t(2) })],
      next_before: 'm2',
      next_after: null,
      trimmed: false,
    });
    s = addOlder(s, {
      messages: [msg({ id: 'm1', created_at: t(1) })],
      next_before: null,
      next_after: null,
      trimmed: false,
    });
    s = upsert(s, msg({ id: 'm4', created_at: t(4) }));
    // Out of order arrival still lands in place.
    s = upsert(s, msg({ id: 'm2b', created_at: t(2) }));
    expect(s.messages.map((m) => m.id)).toEqual(['m4', 'm3', 'm2b', 'm2', 'm1']);
    expect(s.nextBefore).toBeNull();
    expect(newestServerId(s)).toBe('m4');
  });

  it('confirms our own send once, whether the hub answers first or the socket does', () => {
    const local: ChatMessage = {
      ...msg({
        id: 'local-k',
        created_at: t(5),
        sender: { kind: 'user', id: 'usr_1' },
        body: { text: 'hi' },
      }),
      localKey: 'k',
      delivery_status: 'pending',
    };
    const fromHub = msg({
      id: 'm9',
      created_at: t(5),
      sender: { kind: 'user', id: 'usr_1' },
      body: { text: 'hi' },
      delivery_status: 'pending',
    });
    // Socket first, then the reply to the POST.
    let s = addLocal(empty, local);
    expect(newestServerId(s)).toBeNull();
    s = upsert(s, fromHub);
    expect(s.messages.map((m) => m.id)).toEqual(['m9']);
    s = confirmLocal(s, 'k', fromHub);
    expect(s.messages.map((m) => m.id)).toEqual(['m9']);
    // POST first, then the socket.
    s = confirmLocal(addLocal(empty, local), 'k', fromHub);
    s = upsert(s, fromHub);
    expect(s.messages.map((m) => m.id)).toEqual(['m9']);
    // A failed send is not confirmed by an unrelated message with the same words.
    s = upsert(failLocal(addLocal(empty, local), 'k'), { ...fromHub, id: 'm10' });
    expect(s.messages.map((m) => m.id)).toEqual(['local-k', 'm10']);
    expect(s.messages[0]?.delivery_status).toBe('failed');
  });

  it('streams text in, stops typing when the agent speaks, and offers quick replies', () => {
    let s = applyFrame(
      empty,
      {
        type: 'typing',
        data: { conversation_id: 'cnv_1', agent_id: 'agt_1', state: 'start', expires_at: t(30) },
      },
      'cnv_1',
    );
    expect(isTyping(s, Date.parse(t(20)))).toBe(true);
    expect(isTyping(s, Date.parse(t(31)))).toBe(false);
    s = applyFrame(
      s,
      {
        type: 'message.started',
        data: {
          conversation_id: 'cnv_1',
          message: msg({ id: 'm1', created_at: t(1), status: 'streaming', body: {} }),
        },
      },
      'cnv_1',
    );
    expect(isTyping(s, Date.parse(t(20)))).toBe(false);
    s = applyFrame(
      s,
      { type: 'message.delta', data: { conversation_id: 'cnv_1', message_id: 'm1', text: 'Hel' } },
      'cnv_1',
    );
    s = applyFrame(
      s,
      { type: 'message.delta', data: { conversation_id: 'cnv_1', message_id: 'm1', text: 'lo' } },
      'cnv_1',
    );
    expect(s.messages[0]?.body.text).toBe('Hello');
    expect(quickReplies(s)).toEqual([]);
    s = applyFrame(
      s,
      {
        type: 'message.completed',
        data: {
          conversation_id: 'cnv_1',
          message: msg({
            id: 'm1',
            created_at: t(1),
            body: { text: 'Hello', quick_replies: [{ label: 'Hi' }, { label: 'Bye' }] },
          }),
        },
      },
      'cnv_1',
    );
    expect(quickReplies(s)).toEqual(['Hi', 'Bye']);
    // Another conversation's frames are not ours.
    const other = applyFrame(
      s,
      { type: 'message.delta', data: { conversation_id: 'cnv_2', message_id: 'm1', text: '!' } },
      'cnv_1',
    );
    expect(other).toBe(s);
    // Once we answer, the suggestions are gone.
    s = upsert(s, msg({ id: 'm2', created_at: t(2), sender: { kind: 'user', id: 'usr_1' } }));
    expect(quickReplies(s)).toEqual([]);
  });

  it('updates delivery on our messages', () => {
    let s = upsert(
      empty,
      msg({ id: 'm1', created_at: t(1), sender: { kind: 'user', id: 'usr_1' }, delivery_status: 'pending' }),
    );
    s = applyFrame(
      s,
      {
        type: 'delivery.updated',
        data: { conversation_id: 'cnv_1', message_id: 'm1', delivery_status: 'delivered' },
      },
      'cnv_1',
    );
    expect(s.messages[0]?.delivery_status).toBe('delivered');
    expect(appendDelta(s, 'missing', 'x')).toEqual(s);
  });
});

describe('out of my sight', () => {
  it('removes one message and leaves the rest', () => {
    let s = setPage(empty, {
      messages: [
        msg({ id: 'b', created_at: '2026-09-05T10:01:00Z' }),
        msg({ id: 'a', created_at: '2026-09-05T10:00:00Z' }),
      ],
      next_before: 'a',
      next_after: null,
      trimmed: false,
    });
    s = removeMessage(s, 'b');
    expect(s.messages.map((m) => m.id)).toEqual(['a']);
    expect(s.nextBefore).toBe('a');
    expect(removeMessage(s, 'zzz')).toBe(s);
  });

  it('clears everything, with nothing older to page to', () => {
    const s = clearMessages(
      setPage(empty, {
        messages: [msg({ id: 'a', created_at: '2026-09-05T10:00:00Z' })],
        next_before: 'a',
        next_after: null,
        trimmed: false,
      }),
    );
    expect(s.messages).toEqual([]);
    expect(s.nextBefore).toBeNull();
  });
});

describe('the end of history', () => {
  it('says the hub stopped keeping only once the oldest page is on screen', () => {
    let s = setPage(empty, {
      messages: [msg({ id: 'm2', created_at: t(2) })],
      next_before: 'm2',
      next_after: null,
      trimmed: true,
    });
    // More to load: nothing has ended yet.
    expect(historyTrimmed(s)).toBe(false);
    s = addOlder(s, {
      messages: [msg({ id: 'm1', created_at: t(1) })],
      next_before: null,
      next_after: null,
      trimmed: true,
    });
    expect(historyTrimmed(s)).toBe(true);
    // A chat that simply began here says nothing.
    const whole = setPage(empty, {
      messages: [msg({ id: 'm1', created_at: t(1) })],
      next_before: null,
      next_after: null,
      trimmed: false,
    });
    expect(historyTrimmed(whole)).toBe(false);
    // Everything aged out is still the hub's doing, not a new chat.
    expect(
      historyTrimmed(setPage(empty, { messages: [], next_before: null, next_after: null, trimmed: true })),
    ).toBe(true);
    // Cleared by the person: there is nothing left for the note to explain.
    expect(historyTrimmed(clearMessages(s))).toBe(false);
  });
});

describe('scrolled away', () => {
  it('counts what others said since, and not our own', () => {
    const list = setPage(empty, {
      messages: [
        msg({ id: 'e', created_at: '2026-09-05T10:04:00Z' }),
        { ...msg({ id: 'd', created_at: '2026-09-05T10:03:00Z' }), sender: { kind: 'user', id: 'usr_1' } },
        msg({ id: 'c', created_at: '2026-09-05T10:02:00Z' }),
        msg({ id: 'b', created_at: '2026-09-05T10:01:00Z' }),
      ],
      next_before: null,
      next_after: null,
      trimmed: false,
    }).messages;
    expect(unseenSince(list, 'b')).toBe(2);
    expect(unseenSince(list, 'e')).toBe(0);
    expect(unseenSince(list, null)).toBe(0);
  });
});
