import type { Api, CreateAgentInput, SetBindingInput, UpdateAgentInput } from '../api/client';
import type { Agent, Binding } from '../api/types';
import type { Realtime } from '../realtime/realtime';
import { applyFrame, empty, removeAgent, setAgents, upsertAgent, type AgentsState } from './store';

export interface AgentsSnapshot {
  agents: Agent[];
  loading: boolean;
  error: unknown;
}

// AgentsController is the person's agents as a thing outside React: loaded
// from the hub, kept live by the session's connection so the dot beside
// each one and the connect screen's status line change the moment a
// backend attaches. Every change the person makes goes through here too,
// so the list is right without a reload.
export class AgentsController {
  private state: AgentsState = empty;
  private snapshot: AgentsSnapshot = { agents: [], loading: true, error: null };
  private listeners = new Set<() => void>();
  private unsubscribe: (() => void) | null = null;
  private stopped = false;

  constructor(
    private readonly api: Api,
    private readonly realtime: Realtime,
  ) {}

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): AgentsSnapshot => this.snapshot;

  start(): void {
    this.stopped = false;
    void this.refresh();
    this.unsubscribe = this.realtime.subscribe({
      onFrame: (frame) => {
        const { state, stale } = applyFrame(this.state, frame);
        this.set(state);
        if (stale) void this.refresh();
      },
      onOpen: (reconnect) => {
        if (reconnect) void this.refresh();
      },
    });
  }

  stop(): void {
    this.stopped = true;
    this.unsubscribe?.();
    this.unsubscribe = null;
  }

  refresh = async (): Promise<void> => {
    try {
      const list = await this.api.listAgents();
      if (this.stopped) return;
      this.set(setAgents(this.state, list), { error: null, loading: false });
    } catch (error) {
      if (!this.stopped) this.patch({ error, loading: false });
    }
  };

  create = async (input: CreateAgentInput): Promise<Agent> => {
    const agent = await this.api.createAgent(input);
    this.set(upsertAgent(this.state, agent));
    return agent;
  };

  update = async (id: string, input: UpdateAgentInput): Promise<Agent> => {
    const agent = await this.api.updateAgent(id, input);
    this.set(upsertAgent(this.state, agent));
    return agent;
  };

  remove = async (id: string): Promise<void> => {
    await this.api.deleteAgent(id);
    this.set(removeAgent(this.state, id));
  };

  // setBinding returns the secret, which exists nowhere else after this.
  setBinding = async (id: string, input: SetBindingInput): Promise<{ binding: Binding; secret: string }> => {
    const result = await this.api.setBinding(id, input);
    const agent = this.state.agents.find((a) => a.id === id);
    if (agent) this.set(upsertAgent(this.state, { ...agent, binding: result.binding }));
    return result;
  };

  revokeBinding = async (id: string): Promise<void> => {
    await this.api.revokeBinding(id);
    const agent = this.state.agents.find((a) => a.id === id);
    if (agent) this.set(upsertAgent(this.state, { ...agent, binding: null }));
  };

  private set(state: AgentsState, extra: Partial<AgentsSnapshot> = {}): void {
    this.state = state;
    this.patch({ agents: state.agents, ...extra });
  }

  private patch(extra: Partial<AgentsSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...extra };
    for (const l of this.listeners) l();
  }
}
