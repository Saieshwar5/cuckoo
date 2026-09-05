import type { Agent, AgentLiveStatus, Frame } from '../api/types';

// The agents a person owns, and how live announcements change them. Pure,
// so it is tested without a screen.

export interface AgentsState {
  // In the hub's order: oldest first.
  agents: Agent[];
}

export const empty: AgentsState = { agents: [] };

export function setAgents(_: AgentsState, list: Agent[]): AgentsState {
  return { agents: list };
}

export function upsertAgent(state: AgentsState, agent: Agent): AgentsState {
  const i = state.agents.findIndex((a) => a.id === agent.id);
  if (i < 0) return { agents: [...state.agents, agent] };
  const next = state.agents.slice();
  next[i] = agent;
  return { agents: next };
}

export function removeAgent(state: AgentsState, id: string): AgentsState {
  if (!state.agents.some((a) => a.id === id)) return state;
  return { agents: state.agents.filter((a) => a.id !== id) };
}

// applyStatus folds an announcement in. It returns whether the agent was
// found with a binding to update; an agent whose binding we do not know
// about needs a reload, which is the caller's call.
export function applyStatus(
  state: AgentsState,
  id: string,
  status: AgentLiveStatus,
): { state: AgentsState; stale: boolean } {
  const agent = state.agents.find((a) => a.id === id);
  if (!agent) return { state, stale: false };
  if (status === 'none') {
    if (!agent.binding) return { state, stale: false };
    return { state: upsertAgent(state, { ...agent, binding: null }), stale: false };
  }
  if (!agent.binding) return { state, stale: true };
  if (agent.binding.status === status) return { state, stale: false };
  return { state: upsertAgent(state, { ...agent, binding: { ...agent.binding, status } }), stale: false };
}

export function applyFrame(state: AgentsState, frame: Frame): { state: AgentsState; stale: boolean } {
  if (frame.type !== 'agent.status') return { state, stale: false };
  return applyStatus(state, frame.data.agent_id, frame.data.status);
}
