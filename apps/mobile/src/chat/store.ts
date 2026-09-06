import type { DeliveryStatus, Frame, Message, Page } from '../api/types';

// One conversation's messages, and how live frames and our own sends change
// them. Pure, so it is tested without a screen.

// A message as the screen holds it: the hub's, or one of ours the hub has
// not confirmed yet, which carries the key it was sent with so a retry is
// the same send.
export interface ChatMessage extends Message {
  localKey?: string;
}

export interface ChatState {
  // Newest first, the order an inverted list wants.
  messages: ChatMessage[];
  // Cursor for the page before the oldest one loaded; null when history
  // is fully loaded.
  nextBefore: string | null;
  // When the agent's typing indicator expires, or null.
  typingUntil: number | null;
}

export const empty: ChatState = { messages: [], nextBefore: null, typingUntil: null };

// newer orders two messages. At the same instant, one of ours still waiting
// for the hub is the newer: it was typed on this device just now.
function newer(a: ChatMessage, b: ChatMessage): boolean {
  if (a.created_at !== b.created_at) return a.created_at > b.created_at;
  if (!!a.localKey !== !!b.localKey) return !!a.localKey;
  return a.id > b.id;
}

function insert(list: ChatMessage[], m: ChatMessage): ChatMessage[] {
  const i = list.findIndex((x) => !newer(x, m));
  const next = list.slice();
  next.splice(i < 0 ? list.length : i, 0, m);
  return next;
}

function patch(list: ChatMessage[], id: string, fn: (m: ChatMessage) => ChatMessage): ChatMessage[] {
  const i = list.findIndex((m) => m.id === id);
  if (i < 0) return list;
  const next = list.slice();
  next[i] = fn(list[i] as ChatMessage);
  return next;
}

// setPage replaces everything with the latest page of history.
export function setPage(state: ChatState, page: Page): ChatState {
  return { ...state, messages: page.messages, nextBefore: page.next_before };
}

// addOlder appends a page of older history.
export function addOlder(state: ChatState, page: Page): ChatState {
  const known = new Set(state.messages.map((m) => m.id));
  const older = page.messages.filter((m) => !known.has(m.id));
  return { ...state, messages: [...state.messages, ...older], nextBefore: page.next_before };
}

// upsert puts a message from the hub where it belongs. A message from us
// that we sent from this device replaces the unconfirmed copy that was
// waiting for it. Anything from the agent means it is no longer typing.
export function upsert(state: ChatState, m: Message): ChatState {
  const typingUntil = m.sender.kind === 'agent' ? null : state.typingUntil;
  if (state.messages.some((x) => x.id === m.id)) {
    return { ...state, typingUntil, messages: patch(state.messages, m.id, () => m) };
  }
  let list = state.messages;
  if (m.sender.kind === 'user') {
    const j = list.findIndex((x) => x.localKey && x.delivery_status !== 'failed' && sameSend(x, m));
    if (j >= 0) list = list.filter((_, k) => k !== j);
  }
  return { ...state, typingUntil, messages: insert(list, m) };
}

function sameSend(local: ChatMessage, m: Message): boolean {
  if (local.body.action || m.body.action) {
    return local.body.action?.button_id === m.body.action?.button_id;
  }
  return (local.body.text ?? '') === (m.body.text ?? '');
}

export function appendDelta(state: ChatState, id: string, text: string): ChatState {
  return {
    ...state,
    messages: patch(state.messages, id, (m) => ({
      ...m,
      body: { ...m.body, text: (m.body.text ?? '') + text },
    })),
  };
}

export function setDelivery(state: ChatState, id: string, status: DeliveryStatus): ChatState {
  return { ...state, messages: patch(state.messages, id, (m) => ({ ...m, delivery_status: status })) };
}

export function setTyping(state: ChatState, on: boolean, expiresAt: string): ChatState {
  return { ...state, typingUntil: on ? Date.parse(expiresAt) : null };
}

// addLocal puts our own send on screen before the hub has answered.
export function addLocal(state: ChatState, m: ChatMessage): ChatState {
  return { ...state, messages: [m, ...state.messages] };
}

// confirmLocal swaps the unconfirmed copy for what the hub created. If the
// hub's copy already arrived over the socket, the local one just goes.
export function confirmLocal(state: ChatState, key: string, m: Message): ChatState {
  const without = { ...state, messages: state.messages.filter((x) => x.localKey !== key) };
  return upsert(without, m);
}

export function failLocal(state: ChatState, key: string): ChatState {
  return {
    ...state,
    messages: state.messages.map((m) =>
      m.localKey === key ? { ...m, delivery_status: 'failed' as const } : m,
    ),
  };
}

export function retryLocal(state: ChatState, key: string): ChatState {
  return {
    ...state,
    messages: state.messages.map((m) =>
      m.localKey === key ? { ...m, delivery_status: 'pending' as const } : m,
    ),
  };
}

// applyFrame folds one live frame into the conversation. Frames for other
// conversations are not ours to handle.
export function applyFrame(state: ChatState, frame: Frame, conversationId: string): ChatState {
  // Announcements about an agent are the chat list's and the header's
  // business, not the thread's.
  if (frame.type === 'ready' || frame.type === 'agent.status') return state;
  if (frame.data.conversation_id !== conversationId) return state;
  switch (frame.type) {
    case 'message.created':
    case 'message.started':
    case 'message.completed':
      return upsert(state, frame.data.message);
    case 'message.delta':
      return appendDelta(state, frame.data.message_id, frame.data.text);
    case 'delivery.updated':
      return setDelivery(state, frame.data.message_id, frame.data.delivery_status);
    case 'typing':
      return setTyping(state, frame.data.state === 'start', frame.data.expires_at);
  }
}

// newestServerId is the cursor for catching up: the newest message the hub
// gave us, skipping our own unconfirmed ones.
export function newestServerId(state: ChatState): string | null {
  return state.messages.find((m) => !m.localKey)?.id ?? null;
}

// quickReplies are the agent's suggested answers to its latest message,
// offered only while that message is the last word in the conversation.
export function quickReplies(state: ChatState): string[] {
  const last = state.messages[0];
  if (!last || last.sender.kind !== 'agent' || last.status !== 'complete') return [];
  return (last.body.quick_replies ?? []).map((q) => q.label);
}

export function isTyping(state: ChatState, now: number): boolean {
  return state.typingUntil !== null && state.typingUntil > now;
}

// removeMessage takes one message off the screen: the person deleted it
// for themselves. One of ours still waiting for the hub has no id to give
// the hub, so it is not offered for this.
export function removeMessage(state: ChatState, id: string): ChatState {
  if (!state.messages.some((m) => m.id === id)) return state;
  return { ...state, messages: state.messages.filter((m) => m.id !== id) };
}

// clearMessages empties the chat: everything so far is out of sight, and
// there is nothing older to page to.
export function clearMessages(state: ChatState): ChatState {
  return { ...state, messages: [], nextBefore: null };
}
