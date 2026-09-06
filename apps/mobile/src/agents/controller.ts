import type { Api, CreateAgentInput, SetBindingInput, UpdateAgentInput } from '../api/client';
import type { Agent, Binding, Contact, PairAccepted } from '../api/types';
import type { PickedFile } from '../media/pick';
import { keys, MemoryCache, type Cache } from '../cache/cache';
import type { Realtime } from '../realtime/realtime';
import {
  applyFrame,
  empty,
  removeAgent,
  setAgents,
  setBlocked,
  setContacts,
  upsertAgent,
  upsertContact,
  type AgentsState,
} from './store';

export interface AgentsSnapshot {
  agents: Agent[];
  contacts: Contact[];
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
  private snapshot: AgentsSnapshot = { agents: [], contacts: [], loading: true, error: null };
  private listeners = new Set<() => void>();
  private unsubscribe: (() => void) | null = null;
  private stopped = false;
  private readonly cache: Cache;
  private readonly cacheKey: string;

  constructor(
    private readonly api: Api,
    private readonly realtime: Realtime,
    remember: { cache: Cache; userId: string } = { cache: new MemoryCache(), userId: '' },
  ) {
    this.cache = remember.cache;
    this.cacheKey = keys.agents(remember.userId);
  }

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): AgentsSnapshot => this.snapshot;

  start(): void {
    this.stopped = false;
    void this.recall().then(() => this.refresh());
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
      const [list, contacts] = await Promise.all([this.api.listAgents(), this.api.listContacts()]);
      if (this.stopped) return;
      this.set(setContacts(setAgents(this.state, list), contacts), { error: null, loading: false });
    } catch (error) {
      if (!this.stopped) this.patch({ error, loading: false });
    }
  };

  // recall shows the agents as they were last time, before the hub is asked.
  private async recall(): Promise<void> {
    const saved = await this.cache.get<{ agents: Agent[]; contacts: Contact[] }>(this.cacheKey);
    if (this.stopped || !saved || this.state.agents.length || this.state.contacts.length) return;
    this.set(setContacts(setAgents(this.state, saved.agents), saved.contacts), { loading: false });
  }

  // accept takes in the agent behind a scanned code. The list of contacts
  // is reloaded rather than guessed at: the hub's row has the owner's name.
  accept = async (code: string): Promise<PairAccepted> => {
    const result = await this.api.acceptPair(code);
    const contacts = await this.api.listContacts();
    this.set(setContacts(this.state, contacts));
    return result;
  };

  // remember puts a contact on the list without a round trip, for the
  // moment between accepting and the reload.
  remember = (contact: Contact): void => {
    this.set(upsertContact(this.state, contact));
  };

  block = async (agentId: string): Promise<void> => {
    await this.api.blockAgent(agentId);
    this.set(setBlocked(this.state, agentId, true));
  };

  unblock = async (agentId: string): Promise<void> => {
    await this.api.unblockAgent(agentId);
    this.set(setBlocked(this.state, agentId, false));
  };

  create = async (input: CreateAgentInput, picture?: PickedFile | null): Promise<Agent> => {
    const agent = await this.api.createAgent({ ...input, ...(await this.picture(picture)) });
    this.set(upsertAgent(this.state, agent));
    return agent;
  };

  update = async (id: string, input: UpdateAgentInput, picture?: PickedFile | null): Promise<Agent> => {
    const agent = await this.api.updateAgent(id, { ...input, ...(await this.picture(picture)) });
    this.set(upsertAgent(this.state, agent));
    return agent;
  };

  // picture uploads a chosen photo, as part of saving rather than as part
  // of choosing: someone who picks a photo and leaves the screen has not
  // left a stray file on the hub.
  private async picture(file: PickedFile | null | undefined): Promise<{ avatar_media_id?: string }> {
    if (!file) return {};
    const media = await this.api.uploadMedia({
      uri: file.uri,
      name: file.name,
      mimeType: file.mimeType,
    });
    return { avatar_media_id: media.id };
  }

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
    this.patch({ agents: state.agents, contacts: state.contacts, ...extra });
    void this.cache.set(this.cacheKey, { agents: state.agents, contacts: state.contacts });
  }

  private patch(extra: Partial<AgentsSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...extra };
    for (const l of this.listeners) l();
  }
}
