// Wire shapes, as the hub sends them. Field names are the hub's own.

export interface User {
  id: string;
  display_name: string;
  locale: string;
  created_at: string;
  updated_at: string;
}

export interface Verified {
  token: string;
  session: { id: string; expires_at: string };
  user: { id: string; display_name: string; locale: string; created_at: string };
  is_new: boolean;
}

export type ParticipantKind = 'user' | 'agent';
export type AgentStatus = 'idle' | 'connected' | 'unreachable';

export interface Participant {
  kind: ParticipantKind;
  id: string;
  display_name: string;
  handle?: string;
  status?: AgentStatus;
}

export interface Button {
  id: string;
  label: string;
  style: 'default' | 'primary' | 'danger';
}

export interface Body {
  text?: string;
  buttons?: Button[][];
  quick_replies?: { label: string }[];
  action?: { button_id: string; source_message_id: string };
  selected_button_id?: string;
}

export type DeliveryStatus = 'pending' | 'delivered' | 'failed';

export interface Message {
  id: string;
  conversation_id: string;
  sender: { kind: ParticipantKind; id: string };
  body: Body;
  reply_to: { id: string; sender_kind: ParticipantKind; text_preview: string } | null;
  status: 'streaming' | 'complete';
  truncated: boolean;
  delivery_status: DeliveryStatus | null;
  created_at: string;
}

export interface Conversation {
  id: string;
  kind: 'dm' | 'group';
  participants: Participant[];
  last_message: Message | null;
  created_at: string;
}

export interface Page {
  messages: Message[];
  next_before: string | null;
  next_after: string | null;
}

export type BindingMode = 'socket' | 'webhook';

export interface Binding {
  id: string;
  mode: BindingMode;
  webhook_url: string | null;
  status: AgentStatus;
  last_seen_at: string | null;
  created_at: string;
}

export interface Agent {
  id: string;
  handle: string;
  display_name: string;
  description: string;
  created_at: string;
  updated_at: string;
  binding: Binding | null;
}

// What an agent's backend is doing, as the hub announces it: a binding
// status, or none when there is no binding at all.
export type AgentLiveStatus = AgentStatus | 'none';

// An agent as someone deciding to add it sees it.
export interface AgentCard {
  id: string;
  handle: string;
  display_name: string;
  description: string;
  owner: { display_name: string };
  status?: AgentStatus;
  verified: boolean;
}

// What a scanned code resolves to.
export interface PairResolve {
  kind: 'add_agent';
  agent: AgentCard;
  already_added: boolean;
  blocked: boolean;
  conversation_id: string | null;
}

export interface PairAccepted {
  conversation: Conversation;
  new: boolean;
}

// An agent in a person's list.
export interface Contact {
  agent: AgentCard;
  added_via: 'owner' | 'pair_token';
  blocked: boolean;
  conversation_id: string;
  created_at: string;
  agent_deleted: boolean;
}

export interface PairToken {
  id: string;
  kind: 'add_agent';
  payload: unknown;
  max_uses: number | null;
  use_count: number;
  expires_at: string | null;
  created_at: string;
  revoked_at: string | null;
}

// A freshly minted token: the only time the code and the picture exist.
export interface MintedToken {
  token: PairToken;
  code: string;
  url: string;
  qr_png: string;
}

// Frames on the live-update socket.
export type Frame =
  | { type: 'ready'; data: { user_id: string } }
  | {
      type: 'message.created' | 'message.started' | 'message.completed';
      data: { conversation_id: string; message: Message };
    }
  | { type: 'message.delta'; data: { conversation_id: string; message_id: string; text: string } }
  | {
      type: 'delivery.updated';
      data: { conversation_id: string; message_id: string; delivery_status: DeliveryStatus };
    }
  | {
      type: 'typing';
      data: { conversation_id: string; agent_id: string; state: 'start' | 'stop'; expires_at: string };
    }
  | { type: 'agent.status'; data: { agent_id: string; status: AgentLiveStatus } };
