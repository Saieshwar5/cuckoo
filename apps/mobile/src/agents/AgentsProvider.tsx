import React, { createContext, useContext, useEffect, useMemo, useSyncExternalStore } from 'react';

import type { Agent, Contact } from '../api/types';
import { useRealtime } from '../realtime/RealtimeProvider';
import { useSession } from '../session/SessionProvider';
import { AgentsController, type AgentsSnapshot } from './controller';

const AgentsContext = createContext<AgentsController | null>(null);

// AgentsProvider keeps the person's agents alive for the whole signed-in
// session, so the list is there when its tab shows and a profile or connect
// screen can watch one agent change.
export function AgentsProvider({ children }: { children: React.ReactNode }) {
  const { api, token } = useSession();
  const realtime = useRealtime();
  const controller = useMemo(
    () => (token && realtime ? new AgentsController(api, realtime) : null),
    [api, token, realtime],
  );

  useEffect(() => {
    if (!controller) return;
    controller.start();
    return () => controller.stop();
  }, [controller]);

  return <AgentsContext.Provider value={controller}>{children}</AgentsContext.Provider>;
}

const idle: AgentsSnapshot = { agents: [], contacts: [], loading: true, error: null };
const never = () => () => {};
const idleSnapshot = () => idle;

export function useAgentsController(): AgentsController | null {
  return useContext(AgentsContext);
}

// useAgents hands a screen the list and the actions on it.
export function useAgents() {
  const controller = useAgentsController();
  const snapshot = useSyncExternalStore(
    controller?.subscribe ?? never,
    controller?.getSnapshot ?? idleSnapshot,
    controller?.getSnapshot ?? idleSnapshot,
  );
  return { ...snapshot, controller };
}

// useAgent is one agent by id, live: null until the list has loaded, or if
// there is no such agent any more.
export function useAgent(id: string): Agent | null {
  const { agents } = useAgents();
  return agents.find((a) => a.id === id) ?? null;
}

// useContact is the person's link to one agent, owned or added: null until
// the list has loaded, or if they do not have it.
export function useContact(agentId: string): Contact | null {
  const { contacts } = useAgents();
  return contacts.find((c) => c.agent.id === agentId) ?? null;
}
