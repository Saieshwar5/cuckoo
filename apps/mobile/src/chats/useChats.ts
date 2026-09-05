import { useSyncExternalStore } from 'react';

import type { Conversation } from '../api/types';
import { useChatsController } from './ChatsProvider';
import type { ChatsSnapshot } from './controller';

const idle: ChatsSnapshot = { conversations: [], loading: true, error: null, connected: false };
const never = () => () => {};
const idleSnapshot = () => idle;

// useChats hands a screen the chat list, live for as long as the session.
export function useChats() {
  const controller = useChatsController();
  const snapshot = useSyncExternalStore(
    controller?.subscribe ?? never,
    controller?.getSnapshot ?? idleSnapshot,
    controller?.getSnapshot ?? idleSnapshot,
  );
  return { ...snapshot, refresh: controller?.refresh ?? (async () => {}) };
}

// useConversation is one entry of the list, by id: null until the list has
// loaded, or if there is no such chat.
export function useConversation(id: string): Conversation | null {
  const { conversations } = useChats();
  return conversations.find((c) => c.id === id) ?? null;
}
