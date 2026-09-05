import React, { createContext, useContext, useEffect, useMemo } from 'react';

import { useRealtime } from '../realtime/RealtimeProvider';
import { useSession } from '../session/SessionProvider';
import { ChatsController } from './controller';

const ChatsContext = createContext<ChatsController | null>(null);

// ChatsProvider keeps the chat list alive for the whole signed-in session,
// so the list is there the moment its tab shows and the open chat can name
// who it is with.
export function ChatsProvider({ children }: { children: React.ReactNode }) {
  const { api, token } = useSession();
  const realtime = useRealtime();
  const controller = useMemo(
    () => (token && realtime ? new ChatsController(api, realtime) : null),
    [api, token, realtime],
  );

  useEffect(() => {
    if (!controller) return;
    controller.start();
    return () => controller.stop();
  }, [controller]);

  return <ChatsContext.Provider value={controller}>{children}</ChatsContext.Provider>;
}

export function useChatsController(): ChatsController | null {
  return useContext(ChatsContext);
}
