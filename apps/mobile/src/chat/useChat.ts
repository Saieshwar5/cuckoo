import { useEffect, useMemo, useSyncExternalStore } from 'react';

import { useMemory } from '../cache/CacheProvider';
import { useRealtime } from '../realtime/RealtimeProvider';
import { useSession } from '../session/SessionProvider';
import { ChatController, type ChatSnapshot } from './controller';

const idle: ChatSnapshot = {
  messages: [],
  typing: false,
  loading: true,
  loadingOlder: false,
  hasOlder: false,
  trimmed: false,
  connected: false,
  error: null,
  quickReplies: [],
};
const never = () => () => {};
const idleSnapshot = () => idle;
const noop = async () => {};

// useChat hands a screen one conversation and keeps it live for as long as
// the screen is mounted.
export function useChat(conversationId: string) {
  const { api, user } = useSession();
  const realtime = useRealtime();
  const memory = useMemory();
  const userId = user?.id ?? null;
  const controller = useMemo(
    () =>
      realtime && userId && memory
        ? new ChatController(api, realtime, conversationId, userId, Date.now, memory)
        : null,
    [api, realtime, conversationId, userId, memory],
  );

  useEffect(() => {
    if (!controller) return;
    controller.start();
    return () => controller.stop();
  }, [controller]);

  const snapshot = useSyncExternalStore(
    controller?.subscribe ?? never,
    controller?.getSnapshot ?? idleSnapshot,
    controller?.getSnapshot ?? idleSnapshot,
  );
  return {
    ...snapshot,
    send: controller?.send ?? noop,
    retry: controller?.retry ?? noop,
    loadOlder: controller?.loadOlder ?? noop,
    deleteForMe: controller?.deleteForMe ?? noop,
  };
}
