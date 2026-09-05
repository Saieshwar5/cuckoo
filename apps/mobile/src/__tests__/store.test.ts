import type { Conversation, Message } from '@/api/types';
import { applyFrame, empty, setConversations } from '@/chats/store';

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
