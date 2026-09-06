import React, { createContext, useContext, useEffect, useMemo } from 'react';

import { forgetMedia } from '../media/source';
import { Outbox } from '../outbox/outbox';
import { onNetworkBack } from '../realtime/network';
import { useRealtime } from '../realtime/RealtimeProvider';
import { useSession } from '../session/SessionProvider';
import { guarded, type Cache } from './cache';
import { openCache } from './open';

export interface Memory {
  cache: Cache;
  userId: string;
  outbox: Outbox;
}

const MemoryContext = createContext<Memory | null>(null);

// The one store on this device, opened once. A storage failure is logged
// and swallowed: an app that cannot remember is the app as it was, not a
// broken one.
const store = guarded(openCache(), (err) => console.warn('cache unavailable', err));

// CacheProvider gives everything below what it needs to remember and to
// send: the store, whose it is, and the outbox that carries sends to the
// hub whenever the hub can be reached. Present exactly while signed in.
export function CacheProvider({ children }: { children: React.ReactNode }) {
  const { api, token, user } = useSession();
  const realtime = useRealtime();
  const userId = user?.id ?? null;

  const memory = useMemo<Memory | null>(
    () =>
      token && userId && realtime
        ? { cache: store, userId, outbox: new Outbox(api, store, realtime, userId, onNetworkBack) }
        : null,
    [api, token, userId, realtime],
  );

  useEffect(() => {
    if (!memory) return;
    memory.outbox.start();
    return () => memory.outbox.stop();
  }, [memory]);

  return <MemoryContext.Provider value={memory}>{children}</MemoryContext.Provider>;
}

export function useMemory(): Memory | null {
  return useContext(MemoryContext);
}

// forgetEverything is what signing out calls: the documents and the
// pictures both.
export function forgetEverything(): Promise<void> {
  forgetMedia();
  return store.clear();
}
