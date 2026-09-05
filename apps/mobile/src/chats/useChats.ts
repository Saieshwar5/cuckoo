import { useEffect, useMemo, useSyncExternalStore } from 'react';

import { socketUrl } from '../config';
import { useSession } from '../session/SessionProvider';
import { ChatsController } from './controller';

// useChats hands a screen the chat list and keeps it live for as long as the
// screen is mounted.
export function useChats() {
  const { api, token } = useSession();
  const controller = useMemo(() => new ChatsController(api, socketUrl, token ?? ''), [api, token]);

  useEffect(() => {
    if (!token) return;
    controller.start();
    return () => controller.stop();
  }, [controller, token]);

  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  return { ...snapshot, refresh: controller.refresh };
}
