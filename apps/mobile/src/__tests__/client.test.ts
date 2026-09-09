import { ApiError, NetworkError, createApi } from '@/api/client';

function respond(status: number, body: unknown, headers: Record<string, string> = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (k: string) => headers[k] ?? null },
    text: async () => (body === undefined ? '' : JSON.stringify(body)),
  } as unknown as Response;
}

describe('api client', () => {
  it('reads the catalogue and adds from it without a code', async () => {
    const calls: { url: string; method?: string }[] = [];
    const api = createApi({
      baseUrl: 'http://hub',
      getToken: () => 'ses_tok_1',
      fetchImpl: async (url, init) => {
        calls.push({ url: String(url), method: init?.method });
        return String(url).endsWith('/catalogue')
          ? respond(200, { agents: [{ agent: { id: 'agt_1', handle: 'weather' } }] })
          : respond(200, { conversation: { id: 'cnv_1' }, new: true });
      },
    });

    // The listing comes back unwrapped, as a list of cards.
    const listed = await api.listCatalogue();
    expect(listed).toHaveLength(1);
    expect(listed[0]?.agent.handle).toBe('weather');
    expect(calls[0]?.url).toBe('http://hub/v1/client/catalogue');

    // Adding names the agent, not a code: being listed is the invitation.
    const accepted = await api.addFromCatalogue('agt_1');
    expect(accepted.conversation.id).toBe('cnv_1');
    expect(calls[1]?.url).toBe('http://hub/v1/client/catalogue/agt_1/add');
    expect(calls[1]?.method).toBe('POST');
  });

  it('sends the token and an idempotency key, and unwraps envelopes', async () => {
    const calls: { url: string; init: RequestInit }[] = [];
    const api = createApi({
      baseUrl: 'http://hub',
      getToken: () => 'ses_tok_1',
      fetchImpl: async (url, init) => {
        calls.push({ url: String(url), init: init ?? {} });
        return respond(201, { message: { id: 'msg_1' } });
      },
    });
    const m = await api.sendMessage('cnv_1', { text: 'hi' });
    expect(m.id).toBe('msg_1');
    expect(calls[0]?.url).toBe('http://hub/v1/client/conversations/cnv_1/messages');
    const headers = calls[0]?.init.headers as Record<string, string>;
    expect(headers.Authorization).toBe('Bearer ses_tok_1');
    const body = JSON.parse(String(calls[0]?.init.body));
    expect(body.text).toBe('hi');
    expect(body.idempotency_key).toMatch(/^app-/);
  });

  it('turns the error envelope into an ApiError with code, field and retry', async () => {
    const api = createApi({
      baseUrl: 'http://hub',
      getToken: () => null,
      fetchImpl: async () =>
        respond(
          429,
          { error: { code: 'too_many_codes', message: 'Slow down.', field: 'email' } },
          { 'Retry-After': '120' },
        ),
    });
    await expect(api.startSignIn('a@b.com')).rejects.toMatchObject({
      name: 'ApiError',
      status: 429,
      code: 'too_many_codes',
      field: 'email',
      retryAfterSeconds: 120,
    });
  });

  it('tells the session when the hub no longer accepts the token', async () => {
    let unauthorized = 0;
    const api = createApi({
      baseUrl: 'http://hub',
      getToken: () => 'ses_tok_old',
      onUnauthorized: () => unauthorized++,
      fetchImpl: async () => respond(401, { error: { code: 'invalid_credentials', message: 'gone' } }),
    });
    await expect(api.me()).rejects.toBeInstanceOf(ApiError);
    expect(unauthorized).toBe(1);
  });

  it('does not treat a 401 without a token as a lost session', async () => {
    let unauthorized = 0;
    const api = createApi({
      baseUrl: 'http://hub',
      getToken: () => null,
      onUnauthorized: () => unauthorized++,
      fetchImpl: async () => respond(401, { error: { code: 'invalid_code', message: 'no' } }),
    });
    await expect(api.verifySignIn('a@b.com', '000000', 'test')).rejects.toMatchObject({
      code: 'invalid_code',
    });
    expect(unauthorized).toBe(0);
  });

  it('reports an unreachable hub as a NetworkError', async () => {
    const api = createApi({
      baseUrl: 'http://hub',
      getToken: () => null,
      fetchImpl: async () => {
        throw new TypeError('Network request failed');
      },
    });
    await expect(api.listConversations()).rejects.toBeInstanceOf(NetworkError);
  });

  it('lists devices and ends one or all the others', async () => {
    const calls: { method: string; url: string }[] = [];
    const api = createApi({
      baseUrl: 'http://hub',
      getToken: () => 'ses_tok_1',
      fetchImpl: async (url, init) => {
        calls.push({ method: init?.method ?? 'GET', url: String(url) });
        return init?.method === 'DELETE'
          ? respond(204, undefined)
          : respond(200, { devices: [{ id: 'ses_1', name: 'laptop', current: true }] });
      },
    });
    const devices = await api.listDevices();
    expect(devices.map((d) => d.id)).toEqual(['ses_1']);
    await api.signOutDevice('ses_2');
    await api.signOutOtherDevices();
    expect(calls).toEqual([
      { method: 'GET', url: 'http://hub/v1/client/devices' },
      { method: 'DELETE', url: 'http://hub/v1/client/devices/ses_2' },
      { method: 'DELETE', url: 'http://hub/v1/client/devices' },
    ]);
  });

  it('handles empty responses and query parameters', async () => {
    const urls: string[] = [];
    const api = createApi({
      baseUrl: 'http://hub',
      getToken: () => 'ses_tok_1',
      fetchImpl: async (url) => {
        urls.push(String(url));
        return String(url).includes('/messages')
          ? respond(200, { messages: [], next_before: null, next_after: null })
          : respond(204, undefined);
      },
    });
    await expect(api.logout()).resolves.toBeUndefined();
    await api.listMessages('cnv_1', { after: 'msg_9', limit: 50 });
    expect(urls[1]).toBe('http://hub/v1/client/conversations/cnv_1/messages?after=msg_9&limit=50');
  });
});
