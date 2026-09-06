import React, { createContext, useContext, useEffect, useMemo } from 'react';

import { socketUrl } from '../config';
import { useSession } from '../session/SessionProvider';
import { onNetworkBack } from './network';
import { createRealtime, type Realtime } from './realtime';

const RealtimeContext = createContext<Realtime | null>(null);

// RealtimeProvider opens the session's one live connection when there is
// a session and closes it when there is not. Below it, useRealtime() is
// null exactly while signed out.
export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  const { token } = useSession();
  const realtime = useMemo(() => (token ? createRealtime({ url: socketUrl, token }) : null), [token]);

  useEffect(() => {
    if (!realtime) return;
    realtime.start();
    const stopListening = onNetworkBack(() => realtime.retryNow());
    return () => {
      stopListening();
      realtime.stop();
    };
  }, [realtime]);

  return <RealtimeContext.Provider value={realtime}>{children}</RealtimeContext.Provider>;
}

export function useRealtime(): Realtime | null {
  return useContext(RealtimeContext);
}
