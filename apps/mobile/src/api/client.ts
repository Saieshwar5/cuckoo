import type {
  Agent,
  ApiKey,
  Binding,
  Contact,
  Conversation,
  Media,
  Message,
  MintedApiKey,
  MintedToken,
  Page,
  PairAccepted,
  PairResolve,
  PairToken,
  User,
  Verified,
} from './types';

// ApiError is the hub's error envelope as a thrown value. `code` is stable
// and is what the app matches on; `message` is for people.
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly field?: string,
    public readonly retryAfterSeconds?: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

// NetworkError means the hub was never reached.
export class NetworkError extends Error {
  constructor(cause: unknown) {
    super('network');
    this.name = 'NetworkError';
    this.cause = cause;
  }
}

export interface ApiOptions {
  baseUrl: string;
  getToken: () => string | null;
  // Called when the hub says the session is no longer valid.
  onUnauthorized?: () => void;
  fetchImpl?: typeof fetch;
}

export interface SendInput {
  text?: string;
  // Ids of files already uploaded, in the order they should be shown.
  attachments?: string[];
  action?: { button_id: string; source_message_id: string };
  reply_to?: string;
}

// A file this device is about to upload: what a picker gave us.
export interface UploadInput {
  uri: string;
  name: string;
  mimeType: string;
}

export interface CreateAgentInput {
  handle: string;
  display_name: string;
  description: string;
}

export interface UpdateAgentInput {
  display_name?: string;
  description?: string;
}

export interface SetBindingInput {
  mode: 'socket' | 'webhook';
  webhook_url?: string;
}

export interface Api {
  startSignIn(email: string): Promise<void>;
  verifySignIn(email: string, code: string, deviceName: string): Promise<Verified>;
  logout(): Promise<void>;
  me(): Promise<User>;
  updateMe(input: { display_name?: string }): Promise<User>;
  listConversations(): Promise<Conversation[]>;
  listMessages(
    conversationId: string,
    q?: { before?: string; after?: string; limit?: number },
  ): Promise<Page>;
  // The key makes a retry the same send; a caller that will retry keeps it.
  sendMessage(conversationId: string, input: SendInput, idempotencyKey?: string): Promise<Message>;
  // Files. Uploading is its own step, so a slow photo does not hold a
  // message open; the send that follows names what came back.
  uploadMedia(file: UploadInput): Promise<Media>;
  // The address of a file's bytes. Fetching them needs the session token,
  // which is why nothing here is a plain public link.
  mediaUrl(mediaId: string, variant?: 'thumb'): string;
  listAgents(): Promise<Agent[]>;
  getAgent(id: string): Promise<Agent>;
  createAgent(input: CreateAgentInput): Promise<Agent>;
  updateAgent(id: string, input: UpdateAgentInput): Promise<Agent>;
  deleteAgent(id: string): Promise<void>;
  // The secret comes back exactly once.
  setBinding(id: string, input: SetBindingInput): Promise<{ binding: Binding; secret: string }>;
  revokeBinding(id: string): Promise<void>;
  // Handing an agent out, and taking one in.
  createPairToken(
    agentId: string,
    input: { payload?: unknown; max_uses?: number; expires_in?: number },
  ): Promise<MintedToken>;
  listPairTokens(agentId: string): Promise<PairToken[]>;
  revokePairToken(agentId: string, tokenId: string): Promise<void>;
  // The picture of a code this phone kept: the hub renders it only for
  // the code that matches the token.
  pairTokenQR(agentId: string, tokenId: string, code: string): Promise<{ url: string; qr_png: string }>;
  resolvePair(code: string): Promise<PairResolve>;
  acceptPair(code: string): Promise<PairAccepted>;
  listContacts(): Promise<Contact[]>;
  blockAgent(id: string): Promise<void>;
  unblockAgent(id: string): Promise<void>;
  // API keys, for the person's own systems. Issued behind their own
  // credential, so a key can never make another.
  createApiKey(name: string): Promise<MintedApiKey>;
  listApiKeys(): Promise<ApiKey[]>;
  revokeApiKey(id: string): Promise<void>;
}

// newIdempotencyKey is unique enough that two taps of Send never collide,
// which is all an idempotency key has to be.
export function newIdempotencyKey(): string {
  const rand = () => Math.random().toString(16).slice(2, 10);
  return `app-${Date.now().toString(16)}-${rand()}${rand()}`;
}

export function createApi(opts: ApiOptions): Api {
  const fetchImpl = opts.fetchImpl ?? fetch;

  async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    const token = opts.getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
    if (body !== undefined) headers['Content-Type'] = 'application/json';

    let res: Response;
    try {
      res = await fetchImpl(`${opts.baseUrl}${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch (err) {
      throw new NetworkError(err);
    }

    if (res.status === 204) return undefined as T;
    const text = await res.text();
    const json = text ? JSON.parse(text) : {};
    if (!res.ok) {
      const e = json?.error ?? {};
      const retry = res.headers.get('Retry-After');
      if (res.status === 401 && token && opts.onUnauthorized) opts.onUnauthorized();
      throw new ApiError(
        res.status,
        e.code ?? 'unknown',
        e.message ?? 'Request failed.',
        e.field,
        retry ? Number(retry) : undefined,
      );
    }
    return json as T;
  }

  return {
    startSignIn: (email) => request('POST', '/v1/auth/email/start', { email }),
    verifySignIn: (email, code, device_name) =>
      request('POST', '/v1/auth/email/verify', { email, code, device_name }),
    logout: () => request('POST', '/v1/auth/logout'),
    me: async () => (await request<{ user: User }>('GET', '/v1/client/me')).user,
    updateMe: async (input) => (await request<{ user: User }>('PATCH', '/v1/client/me', input)).user,
    listConversations: async () =>
      (await request<{ conversations: Conversation[] }>('GET', '/v1/client/conversations')).conversations,
    listMessages: (id, q = {}) => {
      const params = new URLSearchParams();
      if (q.before) params.set('before', q.before);
      if (q.after) params.set('after', q.after);
      if (q.limit) params.set('limit', String(q.limit));
      const qs = params.toString();
      return request<Page>('GET', `/v1/client/conversations/${id}/messages${qs ? `?${qs}` : ''}`);
    },
    sendMessage: async (id, input, idempotencyKey = newIdempotencyKey()) =>
      (
        await request<{ message: Message }>('POST', `/v1/client/conversations/${id}/messages`, {
          ...input,
          idempotency_key: idempotencyKey,
        })
      ).message,
    uploadMedia: async (file) => {
      const form = new FormData();
      // On the web a picked file is a blob URL; the platform's own File is
      // what fetch can send. On a phone the {uri, name, type} shape is what
      // React Native's fetch understands. Both end up as one multipart part
      // named file, which is all the hub knows about.
      if (file.uri.startsWith('blob:') || file.uri.startsWith('data:')) {
        const blob = await (await fetchImpl(file.uri)).blob();
        form.append('file', new File([blob], file.name, { type: file.mimeType }));
      } else {
        form.append('file', {
          uri: file.uri,
          name: file.name,
          type: file.mimeType,
        } as unknown as Blob);
      }

      const token = opts.getToken();
      let res: Response;
      try {
        res = await fetchImpl(`${opts.baseUrl}/v1/client/media`, {
          method: 'POST',
          headers: token ? { Accept: 'application/json', Authorization: `Bearer ${token}` } : {},
          body: form,
        });
      } catch (err) {
        throw new NetworkError(err);
      }
      const text = await res.text();
      const json = text ? JSON.parse(text) : {};
      if (!res.ok) {
        const e = json?.error ?? {};
        if (res.status === 401 && token && opts.onUnauthorized) opts.onUnauthorized();
        throw new ApiError(res.status, e.code ?? 'unknown', e.message ?? 'Upload failed.', e.field);
      }
      return (json as { media: Media }).media;
    },
    mediaUrl: (mediaId, variant) =>
      `${opts.baseUrl}/v1/client/media/${mediaId}${variant ? `?variant=${variant}` : ''}`,
    listAgents: async () => (await request<{ agents: Agent[] }>('GET', '/v1/mgmt/agents')).agents,
    getAgent: async (id) => (await request<{ agent: Agent }>('GET', `/v1/mgmt/agents/${id}`)).agent,
    createAgent: async (input) => (await request<{ agent: Agent }>('POST', '/v1/mgmt/agents', input)).agent,
    updateAgent: async (id, input) =>
      (await request<{ agent: Agent }>('PATCH', `/v1/mgmt/agents/${id}`, input)).agent,
    deleteAgent: (id) => request('DELETE', `/v1/mgmt/agents/${id}`),
    setBinding: (id, input) =>
      request<{ binding: Binding; secret: string }>('POST', `/v1/mgmt/agents/${id}/binding`, input),
    revokeBinding: (id) => request('DELETE', `/v1/mgmt/agents/${id}/binding`),
    createPairToken: (agentId, input) =>
      request<MintedToken>('POST', `/v1/mgmt/agents/${agentId}/pair-tokens`, input),
    listPairTokens: async (agentId) =>
      (await request<{ tokens: PairToken[] }>('GET', `/v1/mgmt/agents/${agentId}/pair-tokens`)).tokens,
    revokePairToken: (agentId, tokenId) =>
      request('DELETE', `/v1/mgmt/agents/${agentId}/pair-tokens/${tokenId}`),
    pairTokenQR: (agentId, tokenId, code) =>
      request<{ url: string; qr_png: string }>(
        'GET',
        `/v1/mgmt/agents/${agentId}/pair-tokens/${tokenId}/qr?code=${encodeURIComponent(code)}`,
      ),
    resolvePair: (code) => request<PairResolve>('GET', `/v1/client/pair/${encodeURIComponent(code)}`),
    acceptPair: (code) => request<PairAccepted>('POST', `/v1/client/pair/${encodeURIComponent(code)}/accept`),
    listContacts: async () => (await request<{ contacts: Contact[] }>('GET', '/v1/client/contacts')).contacts,
    blockAgent: (id) => request('POST', `/v1/client/agents/${id}/block`),
    unblockAgent: (id) => request('DELETE', `/v1/client/agents/${id}/block`),
    createApiKey: (name) => request<MintedApiKey>('POST', '/v1/client/api-keys', { name }),
    listApiKeys: async () => (await request<{ api_keys: ApiKey[] }>('GET', '/v1/client/api-keys')).api_keys,
    revokeApiKey: (id) => request('DELETE', `/v1/client/api-keys/${id}`),
  };
}
