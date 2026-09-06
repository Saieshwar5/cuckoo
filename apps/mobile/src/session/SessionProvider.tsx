import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

import { createApi, type Api } from '../api/client';
import type { Verified } from '../api/types';
import { forgetEverything } from '../cache/CacheProvider';
import { hubUrl } from '../config';
import { clearSession, loadSession, saveSession, type StoredSession } from './store';

export type SessionStatus = 'loading' | 'signedOut' | 'signedIn';

export interface Session {
  status: SessionStatus;
  token: string | null;
  user: StoredSession['user'] | null;
  api: Api;
  // Called with the hub's answer to a verified code.
  signIn(v: Verified): Promise<void>;
  signOut(): Promise<void>;
  setUser(user: StoredSession['user']): Promise<void>;
}

const SessionContext = createContext<Session | null>(null);

// SessionProvider owns the one credential the app holds and the API client
// bound to it. The client is rebuilt when the token changes, which happens
// exactly at sign-in and sign-out. Everything below asks useSession().
export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<SessionStatus>('loading');
  const [stored, setStored] = useState<StoredSession | null>(null);
  const token = stored?.token ?? null;

  // Signing out, or being signed out, leaves nothing behind: not the
  // credential, and not the conversations remembered under it. A shared
  // phone should not carry someone else's chats.
  const forget = useCallback(async () => {
    setStored(null);
    setStatus('signedOut');
    await Promise.all([clearSession(), forgetEverything()]);
  }, []);

  const api = useMemo(
    () =>
      createApi({
        baseUrl: hubUrl,
        getToken: () => token,
        // The hub said this session is over: signed out elsewhere, expired,
        // or revoked. Drop it and show the sign-in screen.
        onUnauthorized: () => void forget(),
      }),
    [token, forget],
  );

  useEffect(() => {
    let cancelled = false;
    loadSession().then((s) => {
      if (cancelled) return;
      setStored(s);
      setStatus(s ? 'signedIn' : 'signedOut');
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const signIn = useCallback(async (v: Verified) => {
    const s: StoredSession = { token: v.token, user: { id: v.user.id, display_name: v.user.display_name } };
    await saveSession(s);
    setStored(s);
    setStatus('signedIn');
  }, []);

  const signOut = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      // The session is gone from this device either way.
    }
    await forget();
  }, [api, forget]);

  const setUser = useCallback(
    async (user: StoredSession['user']) => {
      if (!token) return;
      const s = { token, user };
      await saveSession(s);
      setStored(s);
    },
    [token],
  );

  const value = useMemo<Session>(
    () => ({ status, token, user: stored?.user ?? null, api, signIn, signOut, setUser }),
    [status, token, stored, api, signIn, signOut, setUser],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const s = useContext(SessionContext);
  if (!s) throw new Error('useSession must be used inside SessionProvider');
  return s;
}
