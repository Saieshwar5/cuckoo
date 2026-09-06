import type { Agent, AgentLiveStatus, Contact, ContactSettings, Frame } from '../api/types';

// The agents a person owns and the ones they added, and how live
// announcements change them. Pure, so it is tested without a screen.

export interface AgentsState {
  // In the hub's order: oldest first.
  agents: Agent[];
  // Newest first, owned ones included with added_via 'owner'.
  contacts: Contact[];
}

export const empty: AgentsState = { agents: [], contacts: [] };

export function setAgents(state: AgentsState, list: Agent[]): AgentsState {
  return { ...state, agents: list };
}

export function setContacts(state: AgentsState, list: Contact[]): AgentsState {
  return { ...state, contacts: list };
}

export function setBlocked(state: AgentsState, agentId: string, blocked: boolean): AgentsState {
  return {
    ...state,
    contacts: state.contacts.map((c) => (c.agent.id === agentId ? { ...c, blocked } : c)),
  };
}

// setSettings folds a change to what the person decided about an agent
// into its row, ahead of the hub confirming it.
export function setSettings(state: AgentsState, agentId: string, settings: ContactSettings): AgentsState {
  return {
    ...state,
    contacts: state.contacts.map((c) => (c.agent.id === agentId ? { ...c, ...settings } : c)),
  };
}

// dropContact takes an added agent out of the list. The agent object
// itself, if owned, is untouched: only an added one can be removed.
export function dropContact(state: AgentsState, agentId: string): AgentsState {
  if (!state.contacts.some((c) => c.agent.id === agentId)) return state;
  return { ...state, contacts: state.contacts.filter((c) => c.agent.id !== agentId) };
}

// isMuted says whether a contact is muted at a moment: muted_until is in
// the future. A mute that ran out is simply over; nothing has to clear it.
export function isMuted(contact: Pick<Contact, 'muted_until'>, now: number = Date.now()): boolean {
  return !!contact.muted_until && Date.parse(contact.muted_until) > now;
}

// upsertContact puts a freshly added agent at the top of the list.
export function upsertContact(state: AgentsState, contact: Contact): AgentsState {
  return { ...state, contacts: [contact, ...state.contacts.filter((c) => c.agent.id !== contact.agent.id)] };
}

export function upsertAgent(state: AgentsState, agent: Agent): AgentsState {
  const i = state.agents.findIndex((a) => a.id === agent.id);
  if (i < 0) return { ...state, agents: [...state.agents, agent] };
  const next = state.agents.slice();
  next[i] = agent;
  return { ...state, agents: next };
}

export function removeAgent(state: AgentsState, id: string): AgentsState {
  if (!state.agents.some((a) => a.id === id) && !state.contacts.some((c) => c.agent.id === id)) return state;
  return {
    agents: state.agents.filter((a) => a.id !== id),
    contacts: state.contacts.filter((c) => c.agent.id !== id),
  };
}

// applyStatus folds an announcement in. It returns whether the agent was
// found with a binding to update; an agent whose binding we do not know
// about needs a reload, which is the caller's call.
export function applyStatus(
  state: AgentsState,
  id: string,
  status: AgentLiveStatus,
): { state: AgentsState; stale: boolean } {
  // The card of an added agent carries the status directly.
  let next = state;
  if (state.contacts.some((c) => c.agent.id === id)) {
    next = {
      ...state,
      contacts: state.contacts.map((c) =>
        c.agent.id === id
          ? { ...c, agent: { ...c.agent, status: status === 'none' ? undefined : status } }
          : c,
      ),
    };
  }
  const agent = next.agents.find((a) => a.id === id);
  if (!agent) return { state: next, stale: false };
  if (status === 'none') {
    if (!agent.binding) return { state: next, stale: false };
    return { state: upsertAgent(next, { ...agent, binding: null }), stale: false };
  }
  if (!agent.binding) return { state: next, stale: true };
  if (agent.binding.status === status) return { state: next, stale: false };
  return { state: upsertAgent(next, { ...agent, binding: { ...agent.binding, status } }), stale: false };
}

export function applyFrame(state: AgentsState, frame: Frame): { state: AgentsState; stale: boolean } {
  if (frame.type !== 'agent.status') return { state, stale: false };
  return applyStatus(state, frame.data.agent_id, frame.data.status);
}
