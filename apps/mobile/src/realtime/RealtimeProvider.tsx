import React, { createContext, useContext, useEffect, useMemo } from 'react';
import { AppState } from 'react-native';

import { socketUrl } from '../config';
import { useSession } from '../session/SessionProvider';
import { onNetworkBack } from './network';
import { createRealtime, type Realtime } from './realtime';

const RealtimeContext = createContext<Realtime | null>(null);

// RealtimeProvider opens the session's one live connection when there is
// a session and closes it when there is not. Below it, useRealtime() is
// null exactly while signed out.
//
// It also closes while the app is in the background. The hub treats a live
// connection as someone looking, and holds back notifications for them; a
// phone in a pocket keeps a socket open for minutes, which would make it
// hold back exactly the notifications that matter — a morning schedule to a
// locked phone. Coming back to the front reconnects and catches up.
export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  const { token } = useSession();
  const realtime = useMemo(() => (token ? createRealtime({ url: socketUrl, token }) : null), [token]);

  useEffect(() => {
    if (!realtime) return;
    realtime.start();
    const stopListening = onNetworkBack(() => realtime.retryNow());
    const appState = AppState.addEventListener('change', (next) => {
      if (next === 'background') realtime.stop();
      else if (next === 'active') realtime.start();
    });
    return () => {
      appState.remove();
      stopListening();
      realtime.stop();
    };
  }, [realtime]);

  return <RealtimeContext.Provider value={realtime}>{children}</RealtimeContext.Provider>;
}

export function useRealtime(): Realtime | null {
  return useContext(RealtimeContext);
}
