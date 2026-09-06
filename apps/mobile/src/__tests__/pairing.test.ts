import type { Contact } from '@/api/types';
import { applyStatus, empty, setBlocked, setContacts, upsertContact } from '@/agents/store';
import { parsePairCode } from '@/util/pairing';

describe('pair codes', () => {
  it('reads the code out of a link, a scheme link, or the bare code', () => {
    expect(parsePairCode('pair_abc12345xyz')).toBe('pair_abc12345xyz');
    expect(parsePairCode('  https://cuckoo.example.com/p/pair_abc12345xyz  ')).toBe('pair_abc12345xyz');
    expect(parsePairCode('http://192.168.1.4:8080/p/pair_abc12345xyz?x=1')).toBe('pair_abc12345xyz');
    expect(parsePairCode('cuckoo://p/pair_abc12345xyz')).toBe('pair_abc12345xyz');
    expect(parsePairCode('https://example.com/other')).toBeNull();
    expect(parsePairCode('pair_short')).toBeNull();
    expect(parsePairCode('')).toBeNull();
  });
});

function contact(id: string, blocked = false): Contact {
  return {
    agent: {
      id,
      handle: id,
      display_name: id,
      description: '',
      owner: { display_name: 'SBI' },
      verified: false,
      status: 'idle',
    },
    added_via: 'pair_token',
    blocked,
    conversation_id: `cnv_${id}`,
    created_at: '2026-09-06T00:00:00Z',
    agent_deleted: false,
  };
}

describe('contacts', () => {
  it('sit beside owned agents, follow status announcements, and remember a block', () => {
    let s = setContacts(empty, [contact('a')]);
    s = upsertContact(s, contact('b'));
    expect(s.contacts.map((c) => c.agent.id)).toEqual(['b', 'a']);
    s = applyStatus(s, 'a', 'connected').state;
    expect(s.contacts[1]?.agent.status).toBe('connected');
    s = applyStatus(s, 'a', 'none').state;
    expect(s.contacts[1]?.agent.status).toBeUndefined();
    s = setBlocked(s, 'b', true);
    expect(s.contacts[0]?.blocked).toBe(true);
    // Re-adding one already there replaces it rather than duplicating.
    s = upsertContact(s, contact('b'));
    expect(s.contacts.filter((c) => c.agent.id === 'b')).toHaveLength(1);
    expect(s.contacts[0]?.blocked).toBe(false);
  });
});
