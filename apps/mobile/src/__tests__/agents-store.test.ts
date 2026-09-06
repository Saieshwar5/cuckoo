import type { Agent, Contact, Conversation } from '@/api/types';
import {
  applyFrame,
  applyStatus,
  dropContact,
  empty,
  isMuted,
  removeAgent,
  setAgents,
  setContacts,
  setSettings,
  upsertAgent,
} from '@/agents/store';
import { applyFrame as applyChatsFrame, setConversations, empty as noChats } from '@/chats/store';

function agent(id: string, status: 'idle' | 'connected' | null): Agent {
  return {
    id,
    handle: id,
    display_name: id,
    description: '',
    created_at: '2026-09-05T00:00:00Z',
    updated_at: '2026-09-05T00:00:00Z',
    binding: status
      ? { id: `bnd_${id}`, mode: 'socket', webhook_url: null, status, last_seen_at: null, created_at: '' }
      : null,
  };
}

describe('agents store', () => {
  it('follows announcements, and asks for a reload when it does not know the binding', () => {
    let s = setAgents(empty, [agent('a', 'idle'), agent('b', null)]);
    let r = applyStatus(s, 'a', 'connected');
    expect(r.state.agents[0]?.binding?.status).toBe('connected');
    expect(r.stale).toBe(false);
    // The same news twice changes nothing.
    expect(applyStatus(r.state, 'a', 'connected').state).toBe(r.state);
    // A binding was made elsewhere: we hold no binding to update.
    r = applyStatus(r.state, 'b', 'idle');
    expect(r.stale).toBe(true);
    // Revoked: the binding goes.
    r = applyStatus(r.state, 'a', 'none');
    expect(r.state.agents[0]?.binding).toBeNull();
    // Unknown agent: nothing.
    expect(applyStatus(r.state, 'zzz', 'connected')).toEqual({ state: r.state, stale: false });
    s = removeAgent(upsertAgent(r.state, agent('c', null)), 'b');
    expect(s.agents.map((a) => a.id)).toEqual(['a', 'c']);
  });

  it('is fed by the same frame the chat list uses for the dot on the avatar', () => {
    const frame = { type: 'agent.status' as const, data: { agent_id: 'a', status: 'connected' as const } };
    expect(applyFrame(setAgents(empty, [agent('a', 'idle')]), frame).state.agents[0]?.binding?.status).toBe(
      'connected',
    );
    const conv: Conversation = {
      id: 'cnv_1',
      kind: 'dm',
      created_at: '',
      last_message: null,
      participants: [
        { kind: 'user', id: 'usr_1', display_name: 'Me' },
        { kind: 'agent', id: 'a', display_name: 'A', status: 'idle' },
      ],
    };
    const chats = applyChatsFrame(setConversations(noChats, [conv]), frame);
    expect(chats.conversations[0]?.participants[1]?.status).toBe('connected');
    const gone = applyChatsFrame(chats, { type: 'agent.status', data: { agent_id: 'a', status: 'none' } });
    expect(gone.conversations[0]?.participants[1]?.status).toBeUndefined();
  });
});

function contact(id: string, over: Partial<Contact> = {}): Contact {
  return {
    agent: { kind: 'agent', id, display_name: id, handle: id, description: '', owner: { display_name: 'o' } },
    added_via: 'pair_token',
    blocked: false,
    conversation_id: `cnv_${id}`,
    created_at: '2026-09-05T00:00:00Z',
    agent_deleted: false,
    muted_until: null,
    pinned: false,
    archived: false,
    ...over,
  } as Contact;
}

describe('what the person decided about an agent', () => {
  it('folds a change into the row and leaves the others alone', () => {
    let s = setContacts(empty, [contact('a'), contact('b')]);
    s = setSettings(s, 'a', { pinned: true });
    s = setSettings(s, 'b', { muted_until: '2200-01-01T00:00:00Z', archived: true });
    expect(s.contacts.map((c) => [c.pinned, c.archived, !!c.muted_until])).toEqual([
      [true, false, false],
      [false, true, true],
    ]);
    // Unknown agent: nothing changes.
    expect(setSettings(s, 'zzz', { pinned: true }).contacts).toEqual(s.contacts);
  });

  it('knows a mute that ran out is over', () => {
    const now = Date.parse('2026-09-06T12:00:00Z');
    expect(isMuted(contact('a'), now)).toBe(false);
    expect(isMuted(contact('a', { muted_until: '2026-09-06T20:00:00Z' }), now)).toBe(true);
    expect(isMuted(contact('a', { muted_until: '2026-09-06T11:59:00Z' }), now)).toBe(false);
  });

  it('drops a removed contact and keeps an owned agent', () => {
    const s = setAgents(setContacts(empty, [contact('a'), contact('mine', { added_via: 'owner' })]), [
      agent('mine', 'idle'),
    ]);
    const next = dropContact(s, 'a');
    expect(next.contacts.map((c) => c.agent.id)).toEqual(['mine']);
    expect(next.agents).toHaveLength(1);
    expect(dropContact(next, 'a')).toBe(next);
  });
});
