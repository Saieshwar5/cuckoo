import type { Agent, Conversation } from '@/api/types';
import { applyFrame, applyStatus, empty, removeAgent, setAgents, upsertAgent } from '@/agents/store';
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
