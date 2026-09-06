import type { Conversation, Frame } from '../api/types';

// The chat list, and how live frames change it. Pure, so it is tested
// without a screen: the same reducer runs on every frame the socket brings.

export interface ChatsState {
  conversations: Conversation[];
}

export const empty: ChatsState = { conversations: [] };

function activity(c: Conversation): string {
  return c.last_message?.created_at ?? c.created_at;
}

function sorted(list: Conversation[]): Conversation[] {
  return [...list].sort((a, b) => (activity(a) < activity(b) ? 1 : activity(a) > activity(b) ? -1 : 0));
}

export function setConversations(_: ChatsState, list: Conversation[]): ChatsState {
  return { conversations: sorted(list) };
}

function update(state: ChatsState, id: string, fn: (c: Conversation) => Conversation): ChatsState {
  const i = state.conversations.findIndex((c) => c.id === id);
  const current = state.conversations[i];
  if (!current) return state;
  const next = state.conversations.slice();
  next[i] = fn(current);
  return { conversations: sorted(next) };
}

// concernsUnknown says whether a frame is about a conversation the list
// does not have: the sign that a reload is due.
export function concernsUnknown(state: ChatsState, frame: Frame): boolean {
  if (frame.type === 'ready' || frame.type === 'agent.status') return false;
  return !state.conversations.some((c) => c.id === frame.data.conversation_id);
}

// applyFrame folds one live frame into the list. A message in a conversation
// we do not know about is ignored; the next refresh brings the conversation.
export function applyFrame(state: ChatsState, frame: Frame): ChatsState {
  switch (frame.type) {
    case 'message.created':
    case 'message.started':
    case 'message.completed': {
      const { conversation_id, message } = frame.data;
      return update(state, conversation_id, (c) => {
        const last = c.last_message;
        if (last && last.id !== message.id && last.created_at > message.created_at) return c;
        return { ...c, last_message: message };
      });
    }
    case 'message.delta': {
      const { conversation_id, message_id, text } = frame.data;
      return update(state, conversation_id, (c) => {
        const last = c.last_message;
        if (!last || last.id !== message_id) return c;
        return {
          ...c,
          last_message: { ...last, body: { ...last.body, text: (last.body.text ?? '') + text } },
        };
      });
    }
    case 'delivery.updated': {
      const { conversation_id, message_id, delivery_status } = frame.data;
      return update(state, conversation_id, (c) => {
        const last = c.last_message;
        if (!last || last.id !== message_id) return c;
        return { ...c, last_message: { ...last, delivery_status } };
      });
    }
    case 'agent.status': {
      const { agent_id, status } = frame.data;
      let changed = false;
      const next = state.conversations.map((c) => {
        if (!c.participants.some((p) => p.kind === 'agent' && p.id === agent_id)) return c;
        changed = true;
        return {
          ...c,
          participants: c.participants.map((p) =>
            p.kind === 'agent' && p.id === agent_id
              ? { ...p, status: status === 'none' ? undefined : status }
              : p,
          ),
        };
      });
      return changed ? { conversations: next } : state;
    }
    default:
      return state;
  }
}

// filterConversations narrows the list to what matches a search: a name,
// a handle, or words in the last message. Empty matches everything.
export function filterConversations(list: Conversation[], query: string): Conversation[] {
  const q = query.trim().toLowerCase();
  if (!q) return list;
  return list.filter(
    (c) =>
      c.participants.some(
        (p) => p.display_name.toLowerCase().includes(q) || (p.handle ?? '').toLowerCase().includes(q),
      ) || (c.last_message?.body.text ?? '').toLowerCase().includes(q),
  );
}
