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

export interface Agent {
  id: string;
  handle: string;
  display_name: string;
  description: string;
  created_at: string;
  binding: { id: string; mode: 'socket' | 'webhook'; status: AgentStatus } | null;
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
    };
